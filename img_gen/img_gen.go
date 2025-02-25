package img_gen

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

const (
	IMGGEN_URI      = "https://api.venice.ai/api/v1/image/generate"
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

	// Progress indicator lines
	PROGRESS_LINES = 28
)

var VeniceDir string
var CurrentUser *user.User

var _wrLog *bufio.Writer
var _lastError string
var _failedCount = 0
var _interrupted bool

// ### TYPES ###
// ========================================================================
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

// ### APP FUNCTIONS ###
// ========================================================================
func checkAPIStatus(uri string, apiKey string) error {
	req, err := http.NewRequest("GET", uri, nil)
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
	initRoundedScale := math.Round(cfgScale*4) / 4
	// Ensure we're within the specified range
	roundedScale := min(max(initRoundedScale, minConfig), maxConfig)

	// Ensure at least some minimal value
	if roundedScale < 1.0 {
		roundedScale = 8.5 // default fallback
	}

	// Format to ensure exactly 2 decimal places and proper 0.25 increments
	return math.Round(roundedScale*4) / 4
}

func initPromptLog(config *PromptConfig) error {
	var promptLogPath string
	promptLogPath = filepath.Join(config.OutputDir, "PromptLog.txt")
	fPromptLog, err := os.Create(promptLogPath)
	if err != nil {
		return err
	}

	_wrLog = bufio.NewWriter(fPromptLog)
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
	for _, logStr := range newStrings {
		_, err := _wrLog.WriteString(logStr)
		if err != nil {
			_wrLog.Flush()
			return err
		}
	}

	_wrLog.Flush()
	return nil
}

func clearErrorDisplay() {
	// Move to the error display area (100 lines below the progress area)
	fmt.Print("\033[100B")
	// Clear 3 lines (adjust as needed)
	for range 3 {
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

func enhancePrompt(basePrompt string, config *PromptConfig, elements *PromptElements) (string, string, string) {
	var enhancementTypes []struct {
		name    string
		items   []string
		enabled bool
	}

	// Define all categories with their corresponding toggles
	// note: Style and Custom are handled independantly
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
	if config.Dirty {
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

	// Separate the "custom" elements from the rest
	var customElements []string
	if config.EnableCustom {
		if len(elements.Custom) > 0 {
			if item := getRandomItem(elements.Custom); item != "" {
				customElements = append(customElements, strings.TrimSpace(item))
				fullPrompt += ", " + strings.Join(customElements, ", ")
			}
		}
	}

	outRandos := strings.Join(randomElements, ", ")
	outCustom := strings.Join(customElements, ", ")
	return fullPrompt, outRandos, outCustom
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
	for range PROGRESS_LINES {
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
	for i := range emojisPerLine {
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
	config, _ := InitializeVeniceConfig()
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

	config.SetDisplaySettings()

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
	fmt.Printf("Custom:   %s\033[K\n", config.DisplayCustom)

	fmt.Printf("\033[K\n")
	fmt.Printf("Failed:   %d\033[K\n", _failedCount)

	// Add error status line
	errorStatus := "None"
	if _lastError != "" {
		errorStatus = _lastError
	}
	fmt.Printf("Error:    %s\033[K\n", errorStatus)

	// ToDo: Add error to output log file if debug is enabled in config
}

func displayError(format string, args ...any) {
	// Clear previous error messages
	clearErrorDisplay()

	// Update lastError
	_lastError = fmt.Sprintf(format, args...)

	// Save cursor position
	fmt.Print("\033[s")

	// Move to error display area
	fmt.Print("\033[100B")

	// Print error
	fmt.Printf("\n❌ ERROR: "+format+"\n", args...)

	// Restore cursor position
	fmt.Print("\033[u")

	// Update the progress display to show the new error
	config, _ := InitializeVeniceConfig()
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
	updatePromptLog([]string{"\n\n❌ ERROR: ", _lastError})

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

func debugLog(format string, args ...any) {
	// Move to line right after progress display
	fmt.Printf("\033[%d;0H", PROGRESS_LINES+1)
	// Clear from cursor to end of line
	fmt.Print("\033[K")
	// Print debug message with timestamp
	fmt.Printf("[%s] %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
	// Return cursor to top for next progress update
	fmt.Print("\033[H")
}

func handleResponse(iRes int, payload *GenerateRequest, config *PromptConfig, client *http.Client, req *http.Request) int {
	maxRetries := 3
	retryDelay := 5 * time.Second

	for retry := range maxRetries {
		if retry > 0 {
			displayError("Retrying request (attempt %d/%d)...", retry+1, maxRetries)
			time.Sleep(retryDelay)
		}

		debugLog("Starting API request...")

		resp, err := client.Do(req)
		if err != nil {
			displayError("HTTP request failed: %v", err)
			debugLog("Request failed")
			_failedCount++
			time.Sleep(10 * time.Second)
			continue
		}
		defer resp.Body.Close()

		debugLog("Got response, reading body...")

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			displayError("Error reading response: %v", err)
			debugLog("Failed to read body")
			_failedCount++
			time.Sleep(10 * time.Second)
			continue
		}

		debugLog("Read body: %d bytes", len(body))

		if resp.StatusCode != 200 {
			var apiError struct {
				Error   string `json:"error"`
				Message string `json:"message"`
				Details any    `json:"details"`
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
				updatePromptLog(append([]string{"Current Prompt: "}, payload.Prompt))
			}

			_failedCount++
			switch resp.StatusCode {
			case 401:
				displayError("Authentication failed - check your API key")
				return iRes
			case 429:
				displayError("Rate limit exceeded - waiting longer before retry")
				time.Sleep(RATE_LIMIT * 2)
				iRes-- // Retry this iteration
			case 500, 502, 503, 504:
				displayError("Server error - will retry")
				time.Sleep(5 * time.Second)
				iRes-- // Retry this iteration
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
		iRes = storeImageResult(iRes, result, payload, config)
		if _lastError != "" {
			debugLog("Stopped due to error writing the image to disk.")
			continue
		}

		debugLog("Completed processing this generation")

		break // Success, exit retry loop
	}

	return iRes
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
			_failedCount++
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
		_lastError = "" // Clear error status on success
	}

	return i
}

// ### THE MEAT n POTATAHS ###
// ========================================================================
func Req_ImgGen() {
	//VeniceDir = vDir
	config, err := InitializeVeniceConfig()
	if err != nil {
		displayError("Initialization failed: %v", err)
		return
	}

	// Set up signal handling at the beginning of main
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		_interrupted = true
		// Clear any pending ANSI commands, flush buffered output, and restore terminal
		fmt.Print("\033[?25h\033[0m") // Show cursor, reset colors
		os.Stdout.Sync()              // Flush any buffered output
		os.Exit(1)
	}()

	configPath := filepath.Join(VeniceDir, "prompt.json")

	if err := checkAPIStatus(IMGGEN_URI, config.APIKey); err != nil {
		displayError("API Status Check Failed: %v", err)
		return
	}

	currentUser, err := user.Current()
	if err != nil {
		displayError("Error getting current user: %v", err)
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
		CfgScale:       1,
		NegativePrompt: config.NegativePrompt,
	}

	var lastCallTime time.Time

	for i := 0; i < config.NumImages; i++ {
		if _interrupted || _failedCount >= 6 {
			// Dump any logged info in the current buffer and break
			_wrLog.Flush()
			break
		}

		if config.Style && len(elements.Style) > 0 {
			style := getRandomItem(elements.Style)
			payload.StylePreset = style
		} else {
			// Ensure StylePreset is empty when style is false
			payload.StylePreset = ""
		}

		if i >= 0 {
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
				newConfig.SetDisplaySettings() // Set display settings after loading config

				payload.CfgScale = newConfig.CfgScale
				payload.NegativePrompt = newConfig.NegativePrompt
				payload.Model = newConfig.Model
				config = &newConfig
			}

			lastCallTime = time.Now()
		}

		fullPrompt, randomElements, customElements := enhancePrompt(config.Prompt, config, elements)
		payload.Prompt = fullPrompt
		if len(payload.Prompt) > MaxPromptLength {
			displayError("Prompt too complex, consider simplifying")
			continue
		}

		payload.Seed = time.Now().UnixNano()%99_999_999 + int64(i)
		if payload.CfgScale == 0 {
			payload.CfgScale = generateCfgScale(config.MinConfig, config.MaxConfig)
		}

		fmt.Print("\033[H")
		updateProgress(
			i,
			config.NumImages,
			payload.StylePreset,
			randomElements+", "+customElements,
			"Generating...",
			payload.Model,
			payload.CfgScale)

		jsonData, err := json.Marshal(payload)
		if err != nil {
			displayError("Error creating request: %v", err)
			continue
		}

		req, err := http.NewRequest("POST", IMGGEN_URI, bytes.NewBuffer(jsonData))
		if err != nil {
			displayError("Error creating HTTP request: %v", err)
			continue
		}

		req.Header.Add("Authorization", "Bearer "+config.APIKey)
		req.Header.Add("Content-Type", "application/json")

		client := &http.Client{Timeout: 60 * time.Second}
		i = handleResponse(i, &payload, config, client, req)
	}

	if !_interrupted {
		// Flush the write buffer to make sure we store any unwritten logged data to our log file.
		_wrLog.Flush()
		// Only clear the screen if not interrupted
		fmt.Print("\033[H\033[2J")
		fmt.Println()
		fmt.Println()
		fmt.Println("✨ Generation complete!")
		fmt.Println()
	}
}
