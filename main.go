package main

import (
	"bufio"
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
	MaxFilenameLen  = 200

	// Available image models
	MODEL_FLUENTLY_XL         = "fluently-xl" // default, fastest
	MODEL_FLUX_DEV            = "flux-dev"    // highest quality
	MODEL_FLUX_DEV_UNCENSORED = "flux-dev-uncensored"
	MODEL_PONY_REALISM        = "pony-realism"         // most uncensored
	MODEL_SDXL                = "lustify-sdxl"         // most gross ...probably
	MODEL_STABLE_DIFFUSION    = "stable-diffusion-3.5" // most creative

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

var wrLog *bufio.Writer

func initPromptLog(config *PromptConfig) error {
	var promptLogPath string
	promptLogPath = filepath.Join(config.OutputDir, "PromptLog.txt")
	fPromptLog, err := os.Create(promptLogPath)
	if err != nil {
		return err
	}

	wrLog = bufio.NewWriter(fPromptLog)
	logLines := []string{
		"Model: " + config.Model,
		fmt.Sprintf("\nImage count: %d", config.NumImages),
		"\nPrompt Name: " + config.PromptName,
		"\nBase Prompt: " + config.Prompt,
		"\n\nBelow are the prompt enhancements for each image result.",
		"\n--------------------------------------------------------------------------------"}
	return updatePromptLog(logLines)
}

func updatePromptLog(newStrings []string) error {
	for i := 0; i < len(newStrings); i++ {
		_, err := wrLog.WriteString(newStrings[i])
		if err != nil {
			//displayError("Error writing %d bytes to Prompt Log\nError: %v", b, err)
			wrLog.Flush()
			return err
		}
	}

	wrLog.Flush()
	return nil
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
	PromptName     string  `json:"prompt_name"`
	NameAsSubDir   bool    `json:"name_as_subdir"`
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

func enhancePrompt(basePrompt string, config *PromptConfig, elements *PromptElements) (string, string, string) {
	var enhancementTypes []struct {
		name    string
		items   []string
		enabled bool
	}

	// Define all categories with their corresponding toggles
	// note: Style and Dirty are handled independantly
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
		{"ACCESSORIES", elements.Accessories, config.EnableAccessories}}

	// Add one random element from each enabled category
	var randomElements []string
	for _, category := range enhancementTypes {
		if category.enabled && len(category.items) > 0 {
			if item := getRandomItem(category.items); item != "" {
				randomElements = append(randomElements, strings.TrimSpace(item))
			}
		}
	}
	// Add "uncensored" to the prompt's random elements if Dirty is enabled
	if config.EnableDirty {
		randomElements = append([]string{"uncensored"}, randomElements...)
	}

	// Now bring everything together into the fullPrompt variable
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

	outRandos := strings.Join(randomElements, ", ")
	outDirty := strings.Join(dirtyElements, ", ")
	return fullPrompt, outRandos, outDirty
}

func getUserAPIKey() (string, error) {
	var newApiKey string
	fmt.Println("This looks like a first-time run - a Venice.ai API key is required to use this utility.")
	fmt.Println("Please provide your API Key (or use [ctrl]+[C] to cancel and come back later)")
	fmt.Println("API Key: ")
	sl := bufio.NewScanner(os.Stdin)
	sl.Scan()
	err := sl.Err()
	if err != nil {
		return newApiKey, err
	}
	newApiKey = sl.Text()
	return newApiKey, nil
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

	// Create template elements.json if it doesn't exist
	elementsPath := filepath.Join(veniceDir, "elements.json")
	if _, err := os.Stat(elementsPath); os.IsNotExist(err) {
		if err := createDefaultElementsFile(elementsPath); err != nil {
			fmt.Printf("Error attempting to create full elements template file. \n")

			fmt.Println("Attempting to create empty elements template (refer docs to populate valid elements manually).")
			// Attempt to create empty elements template if default failed.
			tmplateElements := PromptElements{
				// Base attributes
				Face:     []string{},
				Type:     []string{},
				Hair:     []string{},
				Eyes:     []string{},
				Clothing: []string{},
				Style:    []string{},
				// Extra elements
				Poses:       []string{},
				Accessories: []string{},
				Backgrounds: []string{},
				// Keep dirty the same
				Dirty: []string{},
			}
			elementJSON, err := json.MarshalIndent(tmplateElements, "", "    ")
			if err != nil {
				return nil, fmt.Errorf("error creating template elements: %v", err)
			}
			if err := os.WriteFile(elementsPath, elementJSON, 0644); err != nil {
				return nil, fmt.Errorf("error writing template elements: %v", err)
			}
			fmt.Printf("Created template elements at %s\n", elementsPath)
		}
	}

	// Create template prompt.json if it doesn't exist
	configPath := filepath.Join(veniceDir, "prompt.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Prompt for Venice API Key for first time run.
		newApiKey := "YOUR_API_KEY"
		if newApiKey, err = getUserAPIKey(); err != nil {
			return nil, err
		}

		templateConfig := PromptConfig{
			Model:          MODEL_FLUENTLY_XL,
			APIKey:         newApiKey,
			NegativePrompt: "blur, distort, distorted, blurry, censored, censor, pixelated",
			NumImages:      23,
			MinConfig:      7.5,
			MaxConfig:      15.0,
			Height:         1280,
			Width:          1280,
			Steps:          35,
			Style:          true,

			// Enable/disable features
			EnableFace:        true,
			EnableType:        true,
			EnableHair:        false,
			EnableEyes:        false,
			EnableClothing:    true,
			EnableBackground:  false,
			EnablePoses:       true,
			EnableAccessories: false,
			EnableDirty:       false,

			// Default prompt
			NameAsSubDir: true,
			PromptName:   "Hooded Hacker",
			Prompt:       "a modern hacker wearing a hoodie",
			OutputDir:    filepath.Join(currentUser.HomeDir, "Pictures", "venice"),
		}

		configJSON, err := json.MarshalIndent(templateConfig, "", "    ")
		if err != nil {
			return nil, fmt.Errorf("error creating template config: %v", err)
		}

		if err := os.WriteFile(configPath, configJSON, 0644); err != nil {
			return nil, fmt.Errorf("error writing template config: %v", err)
		}
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
	style string,
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
	fmt.Printf("Style:    %s\033[K\n", style)
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

	// ToDo: Add error to output log file if debug is enabled in config
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
	updateProgress(
		current,
		total,
		"",
		"",
		"Error occurred",
		config.Model,
		config.CfgScale)

	// Set this to only write to log file if debug is set in prompt config
	updatePromptLog([]string{"\n\n❌ ERROR: ", lastError})

	// Pause to allow user to see the error
	time.Sleep(5 * time.Second) // Pause for 5 seconds
}

func getOutputDirectory(config *PromptConfig, currentUser *user.User) (string, bool, error) {
	outputDir := config.OutputDir
	if outputDir == "" {
		outputDir = filepath.Join(currentUser.HomeDir, "Pictures", "venice")
	}

	useSubDir := false
	if config.NameAsSubDir && config.PromptName != "" {
		useSubDir = true
		tmpOutputDir := filepath.Join(outputDir, config.PromptName)

		oPathInfo, err := os.Stat(tmpOutputDir)
		if os.IsNotExist(err) {
			outputDir = tmpOutputDir
		} else {
			if oPathInfo.IsDir() {
				tStamp := time.Now().Unix()
				outputDir = filepath.Join(outputDir, fmt.Sprintf("%s_%d", config.PromptName, tStamp))
			} else {
				outputDir = tmpOutputDir
			}
		}
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", false, err
	}

	return outputDir, useSubDir, nil
}

func generateFilenameAndLogDetail(config *PromptConfig, payload *GenerateRequest, iResult int) string {
	seed := payload.Seed
	cfgScale := payload.CfgScale
	stylePreset := payload.StylePreset
	fullPrompt := payload.Prompt
	promptName := config.PromptName
	basePrompt := config.Prompt
	usingSubDir := config.NameAsSubDir
	outputDir := config.OutputDir
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
		if len(s) > MaxFilenameLen {
			s = s[:MaxFilenameLen]
		}
		return s
	}

	// Create filename with counter to avoid overwrites
	counter := 0
	imgNum := iResult + 1
	iteration := fmt.Sprintf("%d.%d", imgNum, 0)
	nameClean := cleanPrompt(promptName)
	if usingSubDir {
		nameClean = "image"
	}

	var filename string
	var fullFilePath string

	for {
		filename = fmt.Sprintf("%s-%s_seed%d_scale%.1f.png",
			nameClean,
			iteration,
			seed,
			cfgScale,
		)
		fullFilePath = filepath.Join(outputDir, filename)
		if _, err := os.Stat(fullFilePath); os.IsNotExist(err) {
			break // File doesn't exist, we can use this name
		}
		counter++
		iteration = fmt.Sprintf("%d.%d", imgNum, counter)
	}

	enhancedParts := strings.TrimPrefix(fullPrompt, basePrompt)
	enhancedParts = strings.TrimPrefix(enhancedParts, ", ")
	var logLines []string
	logLines = append(logLines, "\n=====> File: ", filename)
	if stylePreset != "" {
		logLines = append(logLines, "\nImage Style: ", stylePreset)
	}
	if enhancedParts != "" {
		logLines = append(logLines, "\nElements:    ", enhancedParts, "\n")
	}
	if err := updatePromptLog(logLines); err != nil {
		return ""
	}

	return fullFilePath
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

func handleResponse(i int, payload *GenerateRequest, config *PromptConfig, client *http.Client, req *http.Request) int {
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
				return i
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

		// Make sure we capture any changes made to the iteration int during attempt to store the image...
		i = storeImageResult(i, result, payload, config)
		if lastError != "" {
			debugLog("Stopped due to error writing the image to disk.")
			continue
		}

		debugLog("Completed processing this generation")

		break // Success, exit retry loop
	}

	return i
}

func storeImageResult(i int, result GenerateResponse, payload *GenerateRequest, config *PromptConfig) int {
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

		filename := generateFilenameAndLogDetail(config, payload, i)
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

	return i
}

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

	outputDir, useSubDir, err := getOutputDirectory(config, currentUser)
	if err != nil {
		displayError("Error creating output directory: %v", err)
		return
	}

	// With all paths and configs set, let's intialize a new TXT file to log the prompts used for each image
	config.OutputDir = outputDir
	if err := initPromptLog(config); err != nil {
		displayError("Error initializing Prompt Log!")
		return
	}

	if config.CfgScale < 1 || config.CfgScale > 20 {
		config.CfgScale = 8.5
	}

	elements, err := loadPromptElements()
	if err != nil {
		displayError("Error loading Elements: %v", err)
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
		if interrupted || failedCount >= 3 {
			// Dump any logged info in the current buffer and break
			wrLog.Flush()
			break
		}

		if config.Style && len(elements.Style) > 0 {
			style := getRandomItem(elements.Style)
			payload.StylePreset = style
		} else {
			// Ensure StylePreset is empty when style is false
			payload.StylePreset = ""
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
				// Re-apply output directory params (determined during initialization) to newConfig
				newConfig.OutputDir = outputDir
				newConfig.NameAsSubDir = useSubDir
				newConfig.setDisplaySettings() // Set display settings after loading config
				payload.Prompt = newConfig.Prompt
				payload.CfgScale = newConfig.CfgScale
				payload.NegativePrompt = newConfig.NegativePrompt
				payload.Model = newConfig.Model
				config = &newConfig

				// When config is reloaded, refresh the random elements
				fullPrompt, randomElements, dirtyElements := enhancePrompt(config.Prompt, config, elements)
				payload.Prompt = fullPrompt

				fmt.Print("\033[H")
				updateProgress(i, config.NumImages,
					payload.StylePreset,
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

		fullPrompt, randomElements, _ := enhancePrompt(config.Prompt, config, elements)
		payload.Prompt = fullPrompt

		fmt.Print("\033[H")
		updateProgress(i, config.NumImages,
			payload.StylePreset,
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
		i = handleResponse(i, &payload, config, client, req)
	}

	if !interrupted {
		// Flush the write buffer to make sure we store any unwritten logged data to our log file.
		wrLog.Flush()
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
    	    "butterfly face design", "scalp texture", "crystal motifs", "blue freckles",
    	    "golden lips", "frost finish", "rainbow designs", "glowing symbols",
    	    "cherry blossom effect", "geometric patterns", "facial gems", "star tattoo",
    	    "shimmering finish", "vine design", "sparkle effects", "tribal face art",
    	    "silver face art", "scalp tattoo", "neck decoration", "black choker",
    	    "leather goggles", "cat-eye pattern", "gold eyes", "cat-eye liner",
    	    "purple eyebrows", "feather headband", "natural freckles", "rosy cheeks",
    	    "shimmering skin tone", "tanned skin tone", "yellow irises", "glowing eye effect",
    	    "snake motif", "eyebrow decoration", "feather ear adornment", "black lips",
    	    "glittery finish on lips", "crystal gems", "light freckles", "eyebrow scar",
    	    "constellation markings", "dragon scale pattern", "dark makeup around eyes",
    	    "face piercings", "moon symbol", "leather eyepatch", "beauty mark",
    	    "war-inspired paint", "bandaged look", "facial scars", "pointed ears design",
    	    "skull motif", "ritual markings", "brass goggles", "silk mask",
    	    "chain veil", "third eye illusion", "rune motifs", "lotus pattern",
    	    "crystal crown", "tribal designs", "crescent motif", "glowing patterns",
    	    "ice crystals", "flame motifs", "leather face covering", "aviator goggles",
    	    "monarch design", "galaxy effect", "scale pattern", "wooden mask",
    	    "fox-inspired mask", "cloth cover for face", "face bandana", "stripe painting",
    	    "dot patterns", "triangle designs"
    	],
    	"type": [
    	    "fur-covered", "many tattoos", "shadow effects on skin", "zebra stripes",
    	    "leopard spots", "tiger-like stripes", "fish-scale texture", "snake scales",
    	    "metallic finish", "crystal-textured appearance", "glowing skin effect",
    	    "stone-like texture", "bark-like texture", "glassy surface", "porcelain look",
    	    "golden sheen", "silver sheen", "bronze tone", "diamond-like skin",
    	    "emerald hue", "ruby shade", "sapphire tint", "jade color",
    	    "marble appearance", "moss-covered texture", "leather-like surface",
    	    "flame motifs", "water patterns", "star designs", "tribal markings",
    	    "geometric configurations", "spiral shapes", "flower arrangements",
    	    "cosmic visuals", "nature-inspired patterns", "wave-like impressions",
    	    "constellation engravings", "cracked texture", "iridescent sheen",
    	    "semi-transparent look", "spotted design", "scale-like pattern", "vine motifs",
    	    "shark skin texture", "leopard-patterned skin", "jaguar-like spots",
    	    "cheetah stripes", "tiger-inspired lines", "zebra markings", "fur layering",
    	    "lion-maned effect", "fox fur texture", "dragonfly wing patterns",
    	    "octopus-like surface", "jellyfish skin design", "dolphin-inspired skin",
    	    "manta ray appearance", "eel texture", "crocodile scale pattern",
    	    "pangolin-like armor", "armadillo shell design", "peacock feather motif",
    	    "raven feather pattern", "phoenix feather arrangement"
    	],
    	"hair": [
    	    "long crimson strands", "black bob with blue tips", "platinum blonde mohawk",
    	    "vivid pink waves", "green pixie style", "dark blue braids",
    	    "purple and blue swirls", "red to orange gradient",
    	    "elongated white straight strands", "rainbow colored short cut",
    	    "shoulder-length purple curls", "neon green locks",
    	    "reddish vintage curls", "blonde box braids", "blue asymmetric bob",
    	    "brown afro style", "rose pink waves", "long dark straight strands",
    	    "white spiky design", "purple to orange transition",
    	    "silver-toned short length", "green twisted hair", "copper curls",
    	    "lavender styled updo", "space buns in silver", "vivid pink mohawk style",
    	    "elongated blue waves", "dual-tone black and red split strands",
    	    "blonde braided updo", "platinum finger waves",
    	    "dark red vintage curls", "neon yellow short length", "purple to silver transition",
    	    "green victory roll style", "rainbow pixie cut", "green mohawk design",
    	    "bronze twisted hair", "pink bob haircut", "blue mohawk design",
    	    "gothic curly look", "short copper strands"
    	],
    	"eyes": [
    	    "sapphire irises", "emerald eyes", "ruby-colored irises",
    	    "amber gaze", "violet eye color", "golden iris hue",
    	    "silver-eyed appearance", "jade-colored irises",
    	    "crimson-looking eyes", "ocean-like eyes", "forest-green vision",
    	    "sunset-tinted irises", "crystal-inspired eyes",
    	    "pearl-like iris look", "copper eye tint", "bronze iris coloration",
    	    "twilight-hued eyes", "midnight-blue gaze", "stormy-eye effect",
    	    "arctic eye tones", "desert-inspired eyes", "misty-eyed appearance",
    	    "lunar eye pattern", "cosmic-inspired irises",
    	    "dragon eye look", "feline eyes", "wolf-like irises",
    	    "hawk-eyed perspective", "fox gaze", "serpent-inspired iris",
    	    "deer-hued vision", "tiger-stripe pupils", "owl-eye design",
    	    "lion-like gaze", "eagle-eyed view", "leopard-tinted iris",
    	    "phoenix eye pattern", "dolphin-esque irises",
    	    "gazelle-like eyes", "lynx-inspired gaze"
    	],
    	"clothing": [
    	    "black leather corset-style outfit", "red silk attire",
    	    "neon bodysuit ensemble", "gothic victorian-inspired wear",
    	    "white wedding-appropriate clothing", "steampunk-themed gear",
    	    "green velvet robe option", "punk denim jacket and vest combination",
    	    "sequined apparel piece", "gothic lolita-styled outfit",
    	    "red qipao dress style", "combat boots footwear",
    	    "fishnet stockings legwear", "vivid pink ensemble",
    	    "purple attire choice", "leather armor wear",
    	    "blue renaissance robe-style clothing", "black latex catsuit design",
    	    "plaid skirt option", "white lace-detailed outfit",
    	    "silver spacesuit-inspired gear", "traditional kimono-style dress",
    	    "black vinyl ensemble", "emerald wrap attire style",
    	    "torn punk jeans fashion", "performance costume",
    	    "leather mini skirt fashion piece", "blue velvet cloak option",
    	    "holographic rave apparel choice", "red hanfu-style outfit",
    	    "black mesh top garment", "white angelic-themed wear",
    	    "purple witchy-inspired attire", "neon cyber-styled gear",
    	    "red vampire-themed clothing", "golden armor wear",
    	    "pastel gothic ensemble style", "leather biker jacket fashion piece",
    	    "fairy outfit selection", "red velvet robe option",
    	    "ice blue robe dress choice", "witch's cloak attire style",
    	    "pink kawaii-inspired wear", "silver bodysuit clothing",
    	    "black cocktail attire style", "cyber punk-styled gear",
    	    "sorceress robe-style ensemble", "steampunk corset choice",
    	    "ninja bodysuit clothing", "tribal warrior armor option",
    	    "racing jumpsuit apparel", "metal armor wear",
    	    "magical outfit selection", "futuristic attire style",
    	    "battle armor gear", "punk rock jacket fashion piece",
    	    "mage robe-style costume"
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
    	    "elegant watch", "simple yet elegant necklace",
    	    "bold and colorful scarf", "stylish hat", "luxurious fur coat",
    	    "vintage-inspired brooch", "eclectic bohemian accessories",
    	    "understated minimalist accessories", "elegant gloves",
    	    "luxurious decadent accessories", "whimsical playful items",
    	    "heroic confident gear", "romantic sentimental trinkets",
    	    "mysterious intriguing adornments",
    	    "vibrant energetic decor", "peaceful serene pieces",
    	    "adventurous bold items", "sophisticated elegant goods"
    	],
    	"backgrounds": [
    	    "sunny beach setting", "bustling city street", "serene mountain vista",
    	    "luxurious mansion backdrop", "vibrant nightclub scene",
    	    "peaceful forest landscape", "dramatic theatrical stage",
    	    "eclectic bohemian-inspired locale", "sophisticated refined environment",
    	    "understated minimalist space", "elegant garden setting",
    	    "luxurious decadent palace", "whimsical playful carnival",
    	    "heroic confident stadium", "romantic sentimental park",
    	    "mysterious intriguing abandoned site", "vibrant energetic concert stage",
    	    "peaceful serene lakeside", "adventurous bold wilderness",
    	    "sophisticated elegant ballroom scene",
    	    "quirky offbeat vintage shop setting", "heroic confident skyscraper backdrop",
    	    "romantic sentimental bridge view", "mysterious intriguing foggy alleyway",
    	    "distressed brick wall with colorful graffiti",
    	    "gritty industrial warehouse scene", "steam-filled subway station platform",
    	    "neon-lit cyberpunk street environment",
    	    "overgrown urban ruins setting", "misty bamboo forest landscape",
    	    "desert oasis with palm trees backdrop",
    	    "snowy mountain peak vista", "hidden jungle waterfall location",
    	    "rustic farmhouse interior", "cozy coffee shop corner",
    	    "ancient stone temple setting", "futuristic space station view",
    	    "rainy city rooftop scene", "seaside boardwalk at sunset",
    	    "retro diner interior backdrop", "tranquil zen garden setting",
    	    "bustling open-air market area",
    	    "historic cobblestone street vista", "modern art gallery environment",
    	    "underground speakeasy scene",
    	    "foggy lighthouse cliff view", "autumn forest path",
    	    "crystal cave interior setting", "vintage train station backdrop"
    	],
    	"dirty": []
    }`
	return os.WriteFile(elementsPath, []byte(defaultElements), 0644)
}
