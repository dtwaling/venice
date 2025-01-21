package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var lastError string

const (
	API_URL         = "https://api.venice.ai/api/v1/image/generate"
	RATE_LIMIT      = 2 * time.Second // Changed to exactly 2 seconds
	emojisPerLine   = 35              // How many emojis fit per line
	MaxPromptLength = 1250

	// Available image models
	MODEL_FLUENTLY_XL         = "fluently-xl" // default, fastest
	MODEL_FLUX_DEV            = "flux-dev"    // highest quality
	MODEL_FLUX_DEV_UNCENSORED = "flux-dev-uncensored"
	MODEL_PONY_REALISM        = "pony-realism" // most uncensored

	// Use a single emoji type for consistency
	DoneBox    = "✅" // or "█" for a solid block
	PendingBox = "⬛" // or "░" for a lighter block
)

func getRandomItem(items []string) string {
	if len(items) == 0 {
		return ""
	}

	// Use crypto/rand to generate index
	var index uint64
	b := make([]byte, 8)
	rand.Read(b)
	index = binary.BigEndian.Uint64(b)

	return items[index%uint64(len(items))]
}

func generateCfgScale(minConfig, maxConfig float64) float64 {
	// Generate random bytes
	b := make([]byte, 8)
	rand.Read(b)

	// Convert to float64 between 0 and 1
	randomValue := float64(binary.BigEndian.Uint64(b)) / float64(math.MaxUint64)

	// Calculate CFG scale
	cfgScale := minConfig + (randomValue * (maxConfig - minConfig))

	// Round to nearest 0.25
	roundedScale := math.Round(cfgScale*4) / 4

	// Ensure we're within the specified range
	if roundedScale < minConfig {
		roundedScale = minConfig
	}
	if roundedScale > maxConfig {
		roundedScale = maxConfig
	}

	// Ensure at least some minimal value
	if roundedScale < 1.0 {
		roundedScale = 8.5 // default fallback
	}

	// Format to ensure exactly 2 decimal places and proper 0.25 increments
	return math.Round(roundedScale*4) / 4
}

// Progress indicator lines
const PROGRESS_LINES = 28

type GenerateRequest struct {
	Model          string  `json:"model"`
	Prompt         string  `json:"prompt"`
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	Steps          int     `json:"steps"`
	HideWatermark  bool    `json:"hide_watermark"`
	ReturnBinary   bool    `json:"return_binary"`
	SafeMode       bool    `json:"safe_mode"`
	CfgScale       float64 `json:"cfg_scale"`
	NegativePrompt string  `json:"negative_prompt"`
	Seed           int64   `json:"seed"`
	StylePreset    string  `json:"style_preset,omitempty"`
}

type GenerateResponse struct {
	Images []string `json:"images"`
}

type PromptConfig struct {
	Model          string  `json:"model"`
	Prompt         string  `json:"prompt"`
	NegativePrompt string  `json:"negative_prompt"`
	NumImages      int     `json:"num_images"`
	OutputDir      string  `json:"output_dir"`
	APIKey         string  `json:"api_key"`
	Style          bool    `json:"style"`
	CfgScale       float64 `json:"cfg_scale"`
	MaxConfig      float64 `json:"max_config"`
	MinConfig      float64 `json:"min_config"`
	Basics         bool    `json:"basics"`
	Extras         bool    `json:"extras"`
	Dirty          bool    `json:"dirty"`
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	Steps          int     `json:"steps"`

	// Individual category toggles
	EnableFace        bool `json:"enable_face"`
	EnableType        bool `json:"enable_type"`
	EnableHair        bool `json:"enable_hair"`
	EnableEyes        bool `json:"enable_eyes"`
	EnableClothing    bool `json:"enable_clothing"`
	EnableBackground  bool `json:"enable_background"`
	EnablePoses       bool `json:"enable_poses"`
	EnableAccessories bool `json:"enable_accessories"`
	EnableDirty       bool `json:"enable_dirty"`

	// Display settings (for progress display)
	DisplayFace        string `json:"display_face,omitempty"`
	DisplayType        string `json:"display_type,omitempty"`
	DisplayHair        string `json:"display_hair,omitempty"`
	DisplayEyes        string `json:"display_eyes,omitempty"`
	DisplayClothing    string `json:"display_clothing,omitempty"`
	DisplayBackground  string `json:"display_background,omitempty"`
	DisplayPoses       string `json:"display_poses,omitempty"`
	DisplayAccessories string `json:"display_accessories,omitempty"`
	DisplayDirty       string `json:"display_dirty,omitempty"`
}

var failedCount = 0

type PromptElements struct {
	// Base attributes
	Face     []string `json:"face"`
	Type     []string `json:"type"`
	Hair     []string `json:"hair"`
	Eyes     []string `json:"eyes"`
	Clothing []string `json:"clothing"`
	Style    []string `json:"style"`

	// Extra elements
	Poses       []string `json:"poses"`
	Accessories []string `json:"accessories"`
	Backgrounds []string `json:"backgrounds"`

	// Keep dirty the same
	Dirty []string `json:"dirty"`
}

func (config *PromptConfig) setDisplaySettings() {
	setDisplay := func(enabled bool) string {
		if enabled {
			return "Enabled"
		}
		return "Disabled"
	}

	config.DisplayFace = setDisplay(config.EnableFace)
	config.DisplayType = setDisplay(config.EnableType)
	config.DisplayHair = setDisplay(config.EnableHair)
	config.DisplayEyes = setDisplay(config.EnableEyes)
	config.DisplayClothing = setDisplay(config.EnableClothing)
	config.DisplayBackground = setDisplay(config.EnableBackground)
	config.DisplayPoses = setDisplay(config.EnablePoses) // Fixed this line
	config.DisplayAccessories = setDisplay(config.EnableAccessories)
	config.DisplayDirty = setDisplay(config.EnableDirty)
}

func clearErrorDisplay() {
	// Move to the error display area (100 lines below the progress area)
	fmt.Print("\033[100B")
	// Clear 3 lines (adjust as needed)
	for i := 0; i < 3; i++ {
		fmt.Print("\033[K\n")
	}
	// Move back to the top
	fmt.Print("\033[100A")
}

func loadPromptElements() (*PromptElements, error) {
	currentUser, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("error getting current user: %v", err)
	}

	elementsPath := filepath.Join(currentUser.HomeDir, ".venice", "elements.json")
	data, err := os.ReadFile(elementsPath)
	if err != nil {
		return nil, fmt.Errorf("error reading elements file: %v", err)
	}

	var elements PromptElements
	if err := json.Unmarshal(data, &elements); err != nil {
		return nil, fmt.Errorf("error parsing elements file: %v", err)
	}

	return &elements, nil
}

func checkAPIStatus(apiKey string) error {
	req, err := http.NewRequest("GET", API_URL, nil)
	if err != nil {
		return fmt.Errorf("error creating health check request: %v", err)
	}

	req.Header.Add("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("API appears to be down: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API health check failed (Status %d): %s",
			resp.StatusCode, string(body))
	}

	return nil
}

func enhancePrompt(basePrompt string, config *PromptConfig, payload *GenerateRequest) (string, string, string) {

	elements, err := loadPromptElements()
	if err != nil {
		return basePrompt, "", ""
	}

	var enhancementTypes []struct {
		name    string
		items   []string
		enabled bool
	}

	// Add style only if style flag is true
	if config.Style && len(elements.Style) > 0 {
		style := getRandomItem(elements.Style)
		payload.StylePreset = style
	} else {
		// Ensure StylePreset is empty when style is false
		payload.StylePreset = ""
	}

	// Define all categories with their corresponding toggles
	enhancementTypes = []struct {
		name    string
		items   []string
		enabled bool
	}{
		{"FACE", elements.Face, config.EnableFace},
		{"TYPE", elements.Type, config.EnableType},
		{"HAIR", elements.Hair, config.EnableHair},
		{"EYES", elements.Eyes, config.EnableEyes},
		{"CLOTHING", elements.Clothing, config.EnableClothing},
		{"BACKGROUND", elements.Backgrounds, config.EnableBackground},
		{"POSES", elements.Poses, config.EnablePoses},
		{"ACCESSORIES", elements.Accessories, config.EnableAccessories},
		{"DIRTY", elements.Dirty, config.EnableDirty},
	}

	// Add style if enabled
	if config.Style && len(elements.Style) > 0 {
		style := getRandomItem(elements.Style)
		payload.StylePreset = style
	}

	// Add one random element from each enabled category
	var randomElements []string
	for _, category := range enhancementTypes {
		if category.enabled && len(category.items) > 0 {
			if item := getRandomItem(category.items); item != "" {
				randomElements = append(randomElements, strings.TrimSpace(item))
			}
		}
	}

	// Add "uncensored" to the prompt if Dirty is enabled
	if config.EnableDirty {
		randomElements = append([]string{"uncensored"}, randomElements...)
	}

	fullPrompt := basePrompt
	if len(randomElements) > 0 {
		if len(basePrompt) > 0 {
			fullPrompt = basePrompt + ", " + strings.Join(randomElements, ", ")
		} else {
			fullPrompt = strings.Join(randomElements, ", ")
		}
	}

	// Separate the "dirty" elements from the rest
	var dirtyElements []string
	if config.EnableDirty {
		dirtyElements = append([]string{"uncensored"}, elements.Dirty...)
	}

	return fullPrompt, strings.Join(randomElements, ", "), strings.Join(dirtyElements, ", ")
}

func initializeVeniceConfig() (*PromptConfig, error) {
	// Get current user's home directory
	currentUser, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("error getting current user: %v", err)
	}

	// Create .venice directory if it doesn't exist
	veniceDir := filepath.Join(currentUser.HomeDir, ".venice")
	if err := os.MkdirAll(veniceDir, 0755); err != nil {
		return nil, fmt.Errorf("error creating .venice directory: %v", err)
	}

	// Create template prompt.json if it doesn't exist
	configPath := filepath.Join(veniceDir, "prompt.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		templateConfig := PromptConfig{
			Model:          MODEL_FLUENTLY_XL,
			APIKey:         "YOUR_API_KEY",
			NegativePrompt: "blur, distort, distorted, blurry, censored, censor, pixelated",
			NumImages:      150,
			MinConfig:      7.5,
			MaxConfig:      15.0,
			Height:         1280,
			Width:          1280,
			Steps:          35,
			Style:          false,

			// Enable/disable features
			EnableFace:        true,
			EnableType:        false,
			EnableHair:        false,
			EnableEyes:        false,
			EnableClothing:    true,
			EnableBackground:  false,
			EnablePoses:       false,
			EnableAccessories: false,
			EnableDirty:       false,

			// Default prompt
			Prompt:    "a modern hacker wearing a hoodie",
			OutputDir: filepath.Join(currentUser.HomeDir, "Pictures", "venice"),
		}

		configJSON, err := json.MarshalIndent(templateConfig, "", "    ")
		if err != nil {
			return nil, fmt.Errorf("error creating template config: %v", err)
		}

		if err := os.WriteFile(configPath, configJSON, 0644); err != nil {
			return nil, fmt.Errorf("error writing template config: %v", err)
		}
		fmt.Printf("Created template config at %s\n", configPath)
		fmt.Println("Please add your API key to the config file and try again")
		return nil, fmt.Errorf("new config file created, needs API key")
	}

	// Rest of the function remains the same...
	promptData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("error reading %s: %v", configPath, err)
	}

	var config PromptConfig
	if err := json.Unmarshal(promptData, &config); err != nil {
		return nil, fmt.Errorf("error parsing prompt.json: %v", err)
	}

	// Check for API key
	if config.APIKey == "" || config.APIKey == "YOUR_API_KEY" {
		return nil, fmt.Errorf("no API key found in config file %s", configPath)
	}

	// Set defaults if not specified
	if config.Width <= 0 {
		config.Width = 1280
	}
	if config.Height <= 0 {
		config.Height = 1280
	}
	if config.Steps <= 5 {
		config.Steps = 5
	}
	if config.Steps > 50 {
		config.Steps = 50
	}

	return &config, nil
}

func updateProgress(current,
	total int,
	elements string,
	status string,
	model string,
	cfg float64) {

	// Move to top
	fmt.Print("\033[H")
	// Clear progress area
	for i := 0; i < PROGRESS_LINES; i++ {
		fmt.Print("\033[K\n")
	}
	// Move back to top
	fmt.Print("\033[H")

	const maxLineWidth = 75
	const indent = "          "
	const numLines = 5

	// Progress percentage
	percentage := int(float64(current+1) / float64(total) * 100)
	fmt.Printf("Progress: [%d/%d] (%d%%)\033[K\n\n", current+1, total, percentage)

	// Print emojis based on emojisPerLine constant
	numFilled := int(float64(percentage) / 100.0 * float64(emojisPerLine))
	for i := 0; i < emojisPerLine; i++ {
		if i < numFilled {
			fmt.Print(DoneBox)
		} else {
			fmt.Print(PendingBox)
		}
	}
	fmt.Print("\033[K\n\n")

	// Status and details
	fmt.Printf("Status:   %s\033[K\n", status)

	// Get the current config to access the base prompt
	config, _ := initializeVeniceConfig()
	basePrompt := config.Prompt

	// Print full prompt
	fmt.Print("Prompt:   ")
	fullPrompt := basePrompt
	if elements != "" {
		if len(basePrompt) > 0 {
			fullPrompt += ", " + elements
		} else {
			fullPrompt = elements
		}
	}

	// Split and format the full prompt across lines
	words := strings.Split(fullPrompt, ", ")
	currentLine := ""
	lineCount := 0

	for i, word := range words {
		testLine := currentLine
		if len(currentLine) > 0 {
			testLine += ", "
		}
		testLine += word

		if len(testLine) > maxLineWidth-10 {
			if len(currentLine) > 0 {
				if lineCount == 0 {
					fmt.Printf("%s\033[K\n", currentLine)
				} else {
					fmt.Printf("%s%s\033[K\n", indent, currentLine)
				}
				lineCount++
				if lineCount >= numLines {
					break
				}
			}
			currentLine = word
		} else {
			if len(currentLine) > 0 {
				currentLine += ", "
			}
			currentLine += word
		}

		if i == len(words)-1 && len(currentLine) > 0 && lineCount < numLines {
			if lineCount == 0 {
				fmt.Printf("%s\033[K\n", currentLine)
			} else {
				fmt.Printf("%s%s\033[K\n", indent, currentLine)
			}
			lineCount++
		}
	}

	// Print remaining empty lines if needed
	for i := lineCount; i < numLines; i++ {
		fmt.Printf("%s\033[K\n", indent)
	}

	config.setDisplaySettings()

	fmt.Printf("\033[K\n")
	fmt.Printf("Model:    %s\033[K\n", model)
	fmt.Printf("Config:   %.2f\033[K\n", math.Round(cfg*4)/4)
	fmt.Printf("Output:   %s\033[K\n", config.OutputDir)
	fmt.Printf("\033[K\n")

	fmt.Printf("Face:     %s\033[K\n", config.DisplayFace)
	fmt.Printf("Type:     %s\033[K\n", config.DisplayType)
	fmt.Printf("Hair:     %s\033[K\n", config.DisplayHair)
	fmt.Printf("Eyes:     %s\033[K\n", config.DisplayEyes)
	fmt.Printf("Clothing: %s\033[K\n", config.DisplayClothing)
	fmt.Printf("Backgrnd: %s\033[K\n", config.DisplayBackground)
	fmt.Printf("Poses:    %s\033[K\n", config.DisplayPoses)
	fmt.Printf("Accesry:  %s\033[K\n", config.DisplayAccessories)
	fmt.Printf("Dirty:    %s\033[K\n", config.DisplayDirty)

	fmt.Printf("\033[K\n")
	fmt.Printf("Failed:   %d\033[K\n", failedCount)

	// Add error status line
	errorStatus := "None"
	if lastError != "" {
		errorStatus = lastError
	}
	fmt.Printf("Error:    %s\033[K\n", errorStatus)
}

func displayError(format string, args ...interface{}) {
	// Clear previous error messages
	clearErrorDisplay()

	// Update lastError
	lastError = fmt.Sprintf(format, args...)

	// Save cursor position
	fmt.Print("\033[s")

	// Move to error display area
	fmt.Print("\033[100B")

	// Print error
	fmt.Printf("\n❌ ERROR: "+format+"\n", args...)

	// Restore cursor position
	fmt.Print("\033[u")

	// Update the progress display to show the new error
	config, _ := initializeVeniceConfig()
	current, total := 0, config.NumImages // Assuming these values are available
	updateProgress(current, total, "",
		"Error occurred", config.Model, config.CfgScale)

	// Pause to allow user to see the error
	time.Sleep(5 * time.Second) // Pause for 5 seconds
}

func generateFilename(outputDir string,
	seed int64,
	fullPrompt,
	basePrompt string,
	cfgScale float64) string {
	// Clean the prompt for filename use
	cleanPrompt := func(prompt string) string {
		// Replace spaces and special characters with underscores
		s := strings.Map(func(r rune) rune {
			switch {
			case r >= 'a' && r <= 'z':
				return r
			case r >= 'A' && r <= 'Z':
				return r
			case r >= '0' && r <= '9':
				return r
			case r == ',':
				return '_' // Explicitly convert commas to underscores
			default:
				return '_'
			}
		}, prompt)

		// Replace multiple consecutive underscores with a single underscore
		for strings.Contains(s, "__") {
			s = strings.ReplaceAll(s, "__", "_")
		}

		// Trim leading/trailing underscores
		s = strings.Trim(s, "_")

		// Limit length to prevent extremely long filenames
		if len(s) > 200 {
			s = s[:200]
		}
		return s
	}

	// Split into base prompt and enhanced elements
	baseClean := cleanPrompt(basePrompt)

	// Get enhanced parts of the prompt
	enhancedParts := strings.TrimPrefix(fullPrompt, basePrompt)
	enhancedParts = strings.TrimPrefix(enhancedParts, ", ")
	elementsClean := cleanPrompt(enhancedParts)

	// Create filename with counter to avoid overwrites
	counter := 1
	var filename string
	for {
		filename = filepath.Join(outputDir,
			fmt.Sprintf("%d_%.1f_%s_%s.png",
				seed,
				cfgScale,
				baseClean,
				elementsClean,
			))
		if _, err := os.Stat(filename); os.IsNotExist(err) {
			break // File doesn't exist, we can use this name
		}
		counter++
		elementsClean = fmt.Sprintf("%s_%d", elementsClean, counter)
	}

	return filename
}

func debugLog(format string, args ...interface{}) {
	// Move to line right after progress display
	fmt.Printf("\033[%d;0H", PROGRESS_LINES+1)
	// Clear from cursor to end of line
	fmt.Print("\033[K")
	// Print debug message with timestamp
	fmt.Printf("[%s] %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
	// Return cursor to top for next progress update
	fmt.Print("\033[H")
}

var interrupted bool

func main() {

	config, err := initializeVeniceConfig()
	if err != nil {
		displayError("Initialization failed: %v", err)
		return
	}

	// Set up signal handling at the beginning of main
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		interrupted = true
		// Clear any pending ANSI commands, flush buffered output, and restore terminal
		fmt.Print("\033[?25h\033[0m") // Show cursor, reset colors
		os.Stdout.Sync()              // Flush any buffered output
		os.Exit(1)
	}()

	configPath := filepath.Join(os.Getenv("HOME"), ".venice", "prompt.json")

	currentUser, err := user.Current()
	if err != nil {
		displayError("Error getting current user: %v", err)
		return
	}

	if err := checkAPIStatus(config.APIKey); err != nil {
		displayError("API Status Check Failed: %v", err)
		return
	}

	outputDir := config.OutputDir
	if outputDir == "" {
		outputDir = filepath.Join(currentUser.HomeDir, "Pictures", "venice")
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		displayError("Error creating output directory: %v", err)
		return
	}

	if config.CfgScale < 1 || config.CfgScale > 20 {
		config.CfgScale = 8.5
	}

	fmt.Print("\033[H\033[2J")
	fmt.Println()
	fmt.Println()

	payload := GenerateRequest{
		Model:          config.Model,
		Prompt:         config.Prompt,
		Width:          config.Width,
		Height:         config.Height,
		Steps:          config.Steps,
		HideWatermark:  true,
		ReturnBinary:   false,
		SafeMode:       false,
		CfgScale:       generateCfgScale(config.MinConfig, config.MaxConfig),
		NegativePrompt: config.NegativePrompt,
	}

	var lastCallTime time.Time

	for i := 0; i < config.NumImages; i++ {
		if interrupted {
			break
		}

		if i > 0 {
			elapsed := time.Since(lastCallTime)
			if sleepDuration := RATE_LIMIT - elapsed; sleepDuration > 0 {
				time.Sleep(sleepDuration)
			}

			if newPromptData, err := os.ReadFile(configPath); err == nil {
				var newConfig PromptConfig
				if err := json.Unmarshal(newPromptData, &newConfig); err != nil {
					displayError("Error parsing updated config: %v", err)
					continue
				}
				newConfig.setDisplaySettings() // Set display settings after loading config
				payload.Prompt = newConfig.Prompt
				payload.CfgScale = newConfig.CfgScale
				payload.NegativePrompt = newConfig.NegativePrompt
				payload.Model = newConfig.Model
				config = &newConfig

				// When config is reloaded, refresh the random elements
				fullPrompt, randomElements, dirtyElements := enhancePrompt(config.Prompt, config, &payload)
				payload.Prompt = fullPrompt

				fmt.Print("\033[H")
				updateProgress(i, config.NumImages,
					randomElements+", "+dirtyElements,
					"Generating...",
					payload.Model,
					payload.CfgScale)

			}

			lastCallTime = time.Now()
		}

		if len(payload.Prompt) > MaxPromptLength {
			displayError("Prompt too complex, consider simplifying")
			continue
		}

		payload.Seed = time.Now().UnixNano()%99_999_999 + int64(i)
		payload.CfgScale = generateCfgScale(config.MinConfig, config.MaxConfig)

		fullPrompt, randomElements, _ := enhancePrompt(config.Prompt, config, &payload)
		payload.Prompt = fullPrompt

		fmt.Print("\033[H")
		updateProgress(i, config.NumImages,
			randomElements,
			"Generating...",
			payload.Model,
			payload.CfgScale)

		jsonData, err := json.Marshal(payload)
		if err != nil {
			displayError("Error creating request: %v", err)
			continue
		}

		req, err := http.NewRequest("POST", API_URL, bytes.NewBuffer(jsonData))
		if err != nil {
			displayError("Error creating HTTP request: %v", err)
			continue
		}

		req.Header.Add("Authorization", "Bearer "+config.APIKey)
		req.Header.Add("Content-Type", "application/json")

		client := &http.Client{Timeout: 60 * time.Second}
		maxRetries := 3
		retryDelay := 5 * time.Second

		for retry := 0; retry < maxRetries; retry++ {
			if retry > 0 {
				displayError("Retrying request (attempt %d/%d)...", retry+1, maxRetries)
				time.Sleep(retryDelay)
			}

			debugLog("Starting API request...")

			resp, err := client.Do(req)
			if err != nil {
				displayError("HTTP request failed: %v", err)
				debugLog("Request failed")
				failedCount++
				time.Sleep(10 * time.Second)
				continue
			}
			defer resp.Body.Close()

			debugLog("Got response, reading body...")

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				displayError("Error reading response: %v", err)
				debugLog("Failed to read body")
				failedCount++
				time.Sleep(10 * time.Second)
				continue
			}

			debugLog("Read body: %d bytes", len(body))

			if resp.StatusCode != 200 {
				var apiError struct {
					Error   string      `json:"error"`
					Message string      `json:"message"`
					Details interface{} `json:"details"`
				}
				if err := json.Unmarshal(body, &apiError); err == nil {
					if apiError.Error != "" {
						displayError("API Error: %s", apiError.Error)
					}
					if apiError.Message != "" {
						displayError("API Message: %s", apiError.Message)
					}
					if apiError.Details != nil {
						displayError("API Details: %v", apiError.Details)
					}
				} else {
					displayError("API Error (Status %d): %s", resp.StatusCode, string(body))
				}

				failedCount++
				switch resp.StatusCode {
				case 401:
					displayError("Authentication failed - check your API key")
					return
				case 429:
					displayError("Rate limit exceeded - waiting longer before retry")
					time.Sleep(RATE_LIMIT * 2)
					i-- // Retry this iteration
				case 500, 502, 503, 504:
					displayError("Server error - will retry")
					time.Sleep(5 * time.Second)
					i-- // Retry this iteration
				default:
					displayError("Unexpected error occurred")
				}
				time.Sleep(10 * time.Second)
				continue
			}

			var result GenerateResponse
			if err := json.Unmarshal(body, &result); err != nil {
				displayError("Error parsing API response: %v", err)
				debugLog("Failed to parse API response")
				continue
			}
			debugLog("Successfully parsed API response, processing %d images", len(result.Images))

			for _, imgData := range result.Images {
				debugLog("Decoding image data...")
				imgBytes, err := base64.StdEncoding.DecodeString(imgData)
				if err != nil {
					displayError("Error decoding image data: %v", err)
					debugLog("Failed to decode image data")
					continue
				}
				debugLog("Successfully decoded image (%d bytes)", len(imgBytes))

				isAllBlack := true
				for _, b := range imgBytes {
					if b != 0 {
						isAllBlack = false
						break
					}
				}

				minImageSize := 100_000
				if isAllBlack {
					displayError("Generated image was all black, retrying...")
					debugLog("Image was all black")
					i--
					continue
				}

				if len(imgBytes) < minImageSize {
					failedCount++
					contentType := http.DetectContentType(imgBytes)
					debugLog("Image too small or wrong format: %s, size: %d", contentType, len(imgBytes))
					if contentType != "image/png" {
						displayError("Unexpected file format: %s (expected PNG)", contentType)
					}
					i--
					continue
				}
				filename := generateFilename(outputDir, payload.Seed, fullPrompt, config.Prompt, payload.CfgScale)
				debugLog("Attempting to save image...")
				debugLog("File size: %d bytes", len(imgBytes))

				if err := os.WriteFile(filename, imgBytes, 0644); err != nil {
					displayError("Error saving image: %v", err)
					debugLog("Failed to save image: %v", err)
					continue
				}

				debugLog("Image Saved Successfully")
				lastError = "" // Clear error status on success
			}

			debugLog("Completed processing this generation")

			break // Success, exit retry loop
		}
	}

	if !interrupted {
		// Only clear the screen if not interrupted
		fmt.Print("\033[H\033[2J")
		fmt.Println()
		fmt.Println()
		fmt.Println("✨ Generation complete!")
		fmt.Println()
	}

}

func createDefaultElementsFile(elementsPath string) error {
	defaultElements := `{
    "style": [
        "3D Model", "Analog Film", "Anime", "Cinematic", "Fantasy Art",
        "Line Art", "Neon Punk", "Origami", "Photographic", "Pixel Art",
        "Texture", "Abstract", "Cubist", "Graffiti", "Hyperrealism",
        "Impressionist", "Renaissance", "Steampunk", "Surrealist", "Typography",
        "Watercolor", "Fighting Game", "Super Mario", "Minecraft", "Pokemon",
        "Retro Arcade", "Retro Game", "RPG Fantasy Game", "Strategy Game",
        "Street Fighter", "Legend of Zelda", "Dreamscape", "Dystopian",
        "Fairy Tale", "Gothic", "Grunge", "Horror", "Minimalist", "Monochrome",
        "Space", "Techwear Fashion", "Tribal", "Alien", "Film Noir", "HDR",
        "Long Exposure", "Neon Noir", "Silhouette", "Tilt-Shift"
    ],
    "face": [
        "butterfly facepaint", "face scales", "crystal markings", "blue freckles",
        "golden lips", "frost makeup", "rainbow facepaint", "glowing symbols",
        "cherry blossom makeup", "geometric makeup", "face gems", "star tattoo",
        "shimmering makeup", "vine tattoo", "sparkle makeup", "tribal facepaint",
        "silver facepaint", "scalp tattoo", "neck tattoo", "black choker",
        "leather goggles", "cat eyes", "gold eyes", "cat eyeliner",
        "purple eyebrows", "feather headband", "natural freckles", "rosy cheeks",
        "shimmering skin", "tanned skin", "yellow eyes", "glowing eye",
        "snake tattoo", "eyebrow piercing", "feather earrings", "black lips",
        "glitter lips", "crystal gems", "light freckles", "eyebrow scar",
        "constellation marks", "dragon scale makeup", "dark eye makeup",
        "face piercings", "moon symbol", "leather eyepatch", "beauty mark",
        "war paint", "bandaged face", "facial scars", "pointed ears",
        "skull paint", "ritual marks", "brass goggles", "silk mask",
        "chain veil", "third eye", "rune marks", "lotus paint",
        "crystal crown", "tribal marks", "crescent tattoo", "glowing marks",
        "ice crystals", "flame marks", "leather mask", "aviator goggles",
        "monarch paint", "galaxy paint", "scale makeup", "wooden mask",
        "fox mask", "cloth mask", "face bandana", "paint stripes",
        "paint dots", "paint triangles"
    ],
    "type": [
        "fur covered", "many tattoos", "shadows on skin", "zebra stripes",
        "leopard spots", "tiger stripes", "fish scales", "snake scales",
        "metallic skin", "crystal skin", "glowing skin", "stone textured",
        "bark textured", "glass skin", "porcelain skin", "golden skin",
        "silver skin", "bronze skin", "diamond skin", "emerald skin",
        "ruby skin", "sapphire skin", "jade skin", "marble skin",
        "moss covered", "leather skin", "flame patterns", "water patterns",
        "star patterns", "tribal markings", "geometric patterns", "spiral patterns",
        "flower patterns", "cosmic patterns", "nature patterns", "wave patterns",
        "constellation markings", "cracked texture", "iridescent", "translucent",
        "spotted pattern", "scaled pattern", "diamond pattern", "vine pattern",
        "shark skin", "leopard spots", "jaguar spots", "cheetah spots",
        "tiger stripes", "zebra stripes", "fur", "lion mane",
        "fox fur", "dragonfly wings", "octopus skin", "jellyfish skin",
        "dolphin skin", "manta ray skin", "eel skin", "crocodile scales",
        "pangolin scales", "armadillo shell", "peacock feathers", "raven feathers",
        "phoenix feathers"
    ],
    "hair": [
        "long crimson hair", "black bob with blue tips", "platinum blonde mohawk",
        "long pink wavy hair", "green pixie cut", "dark blue braided hair",
        "purple and blue swirled hair", "red to orange ombre hair",
        "long white straight hair", "rainbow colored pixie cut",
        "shoulder length purple curls", "neon green dreadlocks",
        "red vintage curled hair", "blonde box braids", "blue asymmetrical bob",
        "brown afro hairstyle", "rose pink wavy hair", "long black straight hair",
        "white spiky hair", "purple to orange hair", "silver short hair",
        "green braided hair", "copper curled hair", "lavender updo hairstyle",
        "silver space buns", "pink mohawk hairstyle", "long blue wavy hair",
        "black and red split hair", "blonde braided updo", "platinum finger waves",
        "dark red vintage curls", "neon yellow short hair", "purple to silver hair",
        "green victory roll hair", "rainbow pixie cut", "bronze braided hair",
        "pink bob cut", "blue mohawk hairstyle", "black gothic curls",
        "short copper hair"
    ],
    "eyes": [
        "sapphire eyes", "emerald eyes", "ruby eyes", "amber eyes",
        "violet eyes", "golden eyes", "silver eyes", "jade eyes",
        "crimson eyes", "ocean eyes", "forest eyes", "sunset eyes",
        "crystal eyes", "pearl eyes", "copper eyes", "bronze eyes",
        "twilight eyes", "midnight eyes", "storm eyes", "arctic eyes",
        "desert eyes", "misty eyes", "lunar eyes", "cosmic eyes",
        "dragon eyes", "feline eyes", "wolf eyes", "hawk eyes",
        "fox eyes", "serpent eyes", "deer eyes", "tiger eyes",
        "owl eyes", "lion eyes", "eagle eyes", "leopard eyes",
        "phoenix eyes", "dolphin eyes", "gazelle eyes", "lynx eyes"
    ],
    "clothing": [
        "black leather corset", "red silk gown", "neon bodysuit",
        "gothic victorian attire", "white wedding attire", "steampunk outfit",
        "green velvet robe", "punk denim vest", "sequin attire",
        "gothic lolita outfit", "red qipao", "combat boots",
        "fishnet stockings", "pink outfit", "purple attire",
        "leather armor", "blue renaissance robe", "black latex catsuit",
        "plaid skirt", "white lace attire", "silver spacesuit",
        "traditional kimono", "black vinyl attire", "emerald wrap attire",
        "torn punk jeans", "show costume", "leather mini skirt",
        "blue velvet cloak", "holographic rave outfit", "red hanfu",
        "black mesh top", "white angel attire", "purple witch attire",
        "neon cyber attire", "red vampire attire", "golden armor",
        "pastel gothic attire", "leather biker jacket", "fairy attire",
        "red velvet robe", "ice blue robe", "witch cloak",
        "pink kawaii attire", "silver bodysuit", "black cocktail attire",
        "cyber punk outfit", "sorceress robe", "steampunk corset",
        "ninja bodysuit", "tribal warrior armor", "racing jumpsuit",
        "metal armor", "magical outfit", "futuristic attire",
        "battle armor", "punk rock jacket", "mage robe"
    ],
    "poses": [
        "confident stance", "relaxed pose", "dramatic gesture",
        "playful expression", "serious demeanor", "sensual curve",
        "athletic pose", "elegant posture", "quirky angle",
        "heroic stance", "romantic gaze", "mysterious profile",
        "vibrant energy", "calm serenity", "dynamic movement",
        "static pose", "intimate closeness", "distant gaze",
        "whimsical expression", "strong and powerful", "soft and delicate",
        "carefree and playful", "moody and introspective",
        "vibrant and energetic", "peaceful and serene",
        "adventurous and bold", "quirky and offbeat",
        "heroic and confident", "mysterious and intriguing"
    ],
    "accessories": [
        "statement piece of jewelry", "designer bag", "fashionable sunglasses",
        "elegant watch", "simple yet elegant necklace", "bold and colorful scarf",
        "stylish hat", "luxurious fur coat", "vintage-inspired brooch",
        "eclectic and bohemian-inspired accessories",
        "understated and minimalist accessories",
        "elegant and refined gloves", "luxurious and decadent accessories",
        "whimsical and playful accessories", "heroic and confident accessories",
        "romantic and sentimental accessories",
        "mysterious and intriguing accessories",
        "vibrant and energetic accessories",
        "peaceful and serene accessories",
        "adventurous and bold accessories",
        "sophisticated and elegant accessories"
    ],
    "backgrounds": [
        "sunny beach", "bustling city street", "serene mountain landscape",
        "luxurious mansion", "vibrant nightclub", "peaceful forest",
        "dramatic and theatrical stage",
        "eclectic and bohemian-inspired setting",
        "sophisticated and refined environment",
        "understated and minimalist space", "elegant and refined garden",
        "luxurious and decadent palace", "whimsical and playful carnival",
        "heroic and confident stadium", "romantic and sentimental park",
        "mysterious and intriguing abandoned building",
        "vibrant and energetic concert", "peaceful and serene lake",
        "adventurous and bold wilderness",
        "sophisticated and elegant ballroom",
        "quirky and offbeat vintage shop",
        "heroic and confident skyscraper",
        "romantic and sentimental bridge",
        "mysterious and intriguing foggy alleyway",
        "distressed brick wall with colorful graffiti",
        "gritty industrial warehouse", "steam-filled subway platform",
        "neon-lit cyberpunk street", "overgrown urban ruins",
        "misty bamboo forest", "desert oasis with palm trees",
        "snowy mountain peak", "hidden jungle waterfall",
        "rustic farmhouse interior", "cozy coffee shop corner",
        "ancient stone temple", "futuristic space station",
        "rainy city rooftop", "seaside boardwalk at sunset",
        "retro diner interior", "tranquil zen garden",
        "bustling open-air market", "historic cobblestone street",
        "modern art gallery", "underground speakeasy",
        "foggy lighthouse cliff", "autumn forest path",
        "crystal cave interior", "vintage train station"
    ],
    "dirty": []
}`
	return os.WriteFile(elementsPath, []byte(defaultElements), 0644)
}
