package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config mirrors the YAML file. yaml tags map field names to keys.
type Config struct {
	APIKey       string   `yaml:"api_key"`
	Sorting      string   `yaml:"sorting"`
	TopRange     string   `yaml:"top_range"`
	Categories   string   `yaml:"categories"`
	AllowSketchy bool     `yaml:"allow_sketchy"`
	AllowNSFW    bool     `yaml:"allow_nsfw"`
	AtLeast      string   `yaml:"atleast"`
	Tags         []string `yaml:"tags"`
	OutputDir    string   `yaml:"output_dir"`
	HistoryFile  string   `yaml:"history_file"`
	Daemon       string   `yaml:"daemon"`
}

// Wallpaper is the subset of the Wallhaven API response we care about.
type Wallpaper struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

type SearchResponse struct {
	Data []Wallpaper `json:"data"`
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	cfg.OutputDir = expandHome(cfg.OutputDir)
	cfg.HistoryFile = expandHome(cfg.HistoryFile)
	return &cfg, nil
}

func buildPurity(cfg *Config) string {
	purity := "1" // sfw always on
	if cfg.AllowSketchy {
		purity += "1"
	} else {
		purity += "0"
	}
	if cfg.AllowNSFW {
		purity += "1"
	} else {
		purity += "0"
	}
	return purity
}

func fetchWallpapers(cfg *Config) ([]Wallpaper, error) {
	params := url.Values{}
	params.Set("sorting", cfg.Sorting)
	params.Set("topRange", cfg.TopRange)
	params.Set("categories", cfg.Categories)
	params.Set("purity", buildPurity(cfg))
	if cfg.AtLeast != "" {
		params.Set("atleast", cfg.AtLeast)
	}
	if len(cfg.Tags) > 0 {
		tagParts := make([]string, len(cfg.Tags))
		for i, tag := range cfg.Tags {
			tagParts[i] = "+" + tag
		}
		params.Set("q", strings.Join(tagParts, " "))
	}
	params.Set("apikey", cfg.APIKey)

	reqURL := "https://wallhaven.cc/api/v1/search?" + params.Encode()
	resp, err := http.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wallhaven returned status %d", resp.StatusCode)
	}

	var result SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return result.Data, nil
}

func loadHistory(path string) map[string]bool {
	history := map[string]bool{}
	data, err := os.ReadFile(path)
	if err != nil {
		return history // no history file yet — that's fine
	}
	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return history
	}
	for _, id := range ids {
		history[id] = true
	}
	return history
}

func saveHistory(path string, history map[string]bool, newID string) error {
	history[newID] = true
	ids := make([]string, 0, len(history))
	for id := range history {
		ids = append(ids, id)
	}
	// cap history size at 200
	if len(ids) > 200 {
		ids = ids[len(ids)-200:]
	}
	data, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func pickWallpaper(wallpapers []Wallpaper, history map[string]bool) Wallpaper {
	candidates := []Wallpaper{}
	for _, w := range wallpapers {
		if !history[w.ID] {
			candidates = append(candidates, w)
		}
	}
	if len(candidates) == 0 {
		candidates = wallpapers // all seen before — just pick from the full set
	}
	return candidates[rand.Intn(len(candidates))]
}

func downloadWallpaper(w Wallpaper, outputDir string) (string, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", err
	}
	resp, err := http.Get(w.Path)
	if err != nil {
		return "", fmt.Errorf("downloading wallpaper: %w", err)
	}
	defer resp.Body.Close()

	filename := filepath.Base(w.Path)
	target := filepath.Join(outputDir, filename)

	out, err := os.Create(target)
	if err != nil {
		return "", err
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return "", err
	}
	return target, nil
}

func setWallpaper(path string, daemon string) {
	if daemon == "cosmic-bg" {
		exec.Command("killall", "cosmic-bg").Run()
		cmd := exec.Command("sh", "-c", fmt.Sprintf(
			"sed -i 's|Path(\".*\")|Path(\"%s\")|' ~/.config/cosmic/com.system76.CosmicBackground/v1/all && cosmic-bg &",
			path,
		))
		cmd.Run()
	}
}

func Notify(title, messsage string) {
	exec.Command("notify-send", title, messsage).Run()
}

func main() {
	home, _ := os.UserHomeDir()
	defaultConfigPath := filepath.Join(home, ".config", "autohaven", "config.yaml")

	configFlag := flag.String("config", "", "Path to the YAML configuration file")
	daemonFlag := flag.String("daemon", "", "Override the desktop wallpaper daemon (e.g., cosmic-bg)")
	dryRunFlag := flag.Bool("dry-run", false, "Simulate fetching and picking without downloading or changing wallpaper")
	tagsFlag := flag.String("tag", "", "Override config tags with a comma-separated list of tags")
	noNotifyFlag := flag.Bool("no-notify", false, "Disable desktop notifications")
	cleanFlag := flag.Bool("clean", false, "Clear the history file and downloaded cache/output directory, then exit")
	flag.Parse()

	configPath := defaultConfigPath
	if *configFlag != "" {
		configPath = *configFlag
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	// Handle cleaning option if requested
	if *cleanFlag {
		fmt.Println("Cleaning cache and history...")
		if err := os.Remove(cfg.HistoryFile); err != nil && !os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "warning: failed to delete history file:", err)
		} else {
			fmt.Println("Deleted history file:", cfg.HistoryFile)
		}
		if err := os.RemoveAll(cfg.OutputDir); err != nil {
			fmt.Fprintln(os.Stderr, "warning: failed to delete output directory:", err)
		} else {
			fmt.Println("Deleted cache/output directory:", cfg.OutputDir)
		}
		os.Exit(0)
	}

	// Overrides from flags
	if *daemonFlag != "" {
		cfg.Daemon = *daemonFlag
	}
	if *tagsFlag != "" {
		tagParts := strings.Split(*tagsFlag, ",")
		for i, t := range tagParts {
			tagParts[i] = strings.TrimSpace(t)
		}
		cfg.Tags = tagParts
	}

	wallpapers, err := fetchWallpapers(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if len(wallpapers) == 0 {
		fmt.Fprintln(os.Stderr, "error: no wallpapers returned")
		os.Exit(1)
	}

	history := loadHistory(cfg.HistoryFile)
	chosen := pickWallpaper(wallpapers, history)

	if *dryRunFlag {
		fmt.Printf("Dry-run mode. Selected wallpaper:\n- ID:  %s\n- URL: %s\n", chosen.ID, chosen.Path)
		return
	}

	target, err := downloadWallpaper(chosen, cfg.OutputDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	if err := saveHistory(cfg.HistoryFile, history, chosen.ID); err != nil {
		fmt.Fprintln(os.Stderr, "warning: couldn't save history:", err)
	}

	fmt.Println("Downloaded:", target)

	if cfg.Daemon != "" && cfg.Daemon != "none" {
		setWallpaper(target, cfg.Daemon)
		fmt.Println("Wallpaper set via", cfg.Daemon)
	}

	if !*noNotifyFlag {
		Notify("Autohaven", "Wallpaper set to "+chosen.Path)
	}
}
