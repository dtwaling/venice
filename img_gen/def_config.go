package img_gen

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

var _promptConfigPath string

type PromptConfig struct {
	Model          string  `json:"model"`
	PromptName     string  `json:"prompt_name"`
	NameAsSubDir   bool    `json:"name_as_subdir"`
	Prompt         string  `json:"prompt"`
	NegativePrompt string  `json:"negative_prompt"`
	NumImages      int     `json:"num_images"`
	OutputDir      string  `json:"output_dir"`
	APIKey         string  `json:"api_key"`
	CfgScale       float64 `json:"cfg_scale"`
	MaxConfig      float64 `json:"max_config"`
	MinConfig      float64 `json:"min_config"`
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	Steps          int     `json:"steps"`

	// Style related settings
	Style  bool `json:"style"`
	Basics bool `json:"basics"`
	Extras bool `json:"extras"`
	Dirty  bool `json:"dirty"`

	// Individual category toggles
	EnableFace        bool `json:"enable_face"`
	EnableType        bool `json:"enable_type"`
	EnableHair        bool `json:"enable_hair"`
	EnableEyes        bool `json:"enable_eyes"`
	EnableClothing    bool `json:"enable_clothing"`
	EnableBackground  bool `json:"enable_background"`
	EnablePoses       bool `json:"enable_poses"`
	EnableAccessories bool `json:"enable_accessories"`
	EnableCustom      bool `json:"enable_custom"`

	// Display settings (for progress display)
	DisplayFace        string `json:"display_face,omitempty"`
	DisplayType        string `json:"display_type,omitempty"`
	DisplayHair        string `json:"display_hair,omitempty"`
	DisplayEyes        string `json:"display_eyes,omitempty"`
	DisplayClothing    string `json:"display_clothing,omitempty"`
	DisplayBackground  string `json:"display_background,omitempty"`
	DisplayPoses       string `json:"display_poses,omitempty"`
	DisplayAccessories string `json:"display_accessories,omitempty"`
	DisplayCustom      string `json:"display_custom,omitempty"`
}

// ##- PromptConfig extension methods -##
func (config *PromptConfig) SetDisplaySettings() {
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
	config.DisplayCustom = setDisplay(config.EnableCustom)
}

// ### CONFIG HELPER METHODS ###
// ========================================================================
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

func initPromptConfig(newInitOnly bool) (*PromptConfig, error) {
	// Set global config path variable
	_promptConfigPath = filepath.Join(VeniceDir, "prompt.json")
	var lConfig PromptConfig

	// Use common local functions to reduce duplicate code.
	writePromptConfig := func(apiKey string) error {
		if apiKey == "" {
			// Prompt for Venice API Key for first time run.
			if newApiKey, err := getUserAPIKey(); err != nil {
				return err
			} else {
				apiKey = newApiKey
			}
		}

		templateConfig := PromptConfig{
			Model:          MODEL_FLUX_DEV,
			APIKey:         apiKey,
			NegativePrompt: "blur, distort, distorted, blurry, censored, censor, pixelated",
			NumImages:      42,
			MinConfig:      7.5,
			MaxConfig:      15.0,
			Height:         1280,
			Width:          1280,
			Steps:          30,
			Style:          true,

			// Enable/disable features
			EnableFace:        false,
			EnableType:        false,
			EnableHair:        false,
			EnableEyes:        false,
			EnableClothing:    false,
			EnableBackground:  true,
			EnablePoses:       false,
			EnableAccessories: false,
			EnableCustom:      true,

			// Default prompt
			NameAsSubDir: true,
			PromptName:   "Hot Rod Legends",
			Prompt:       "A legendary drag race between two super-charged hot rods",
			OutputDir:    filepath.Join(CurrentUser.HomeDir, "Pictures", "venice"),
		}
		configJSON, err := json.MarshalIndent(templateConfig, "", "    ")
		if err != nil {
			return fmt.Errorf("error creating template config: %v", err)
		}
		if err := os.WriteFile(_promptConfigPath, configJSON, 0644); err != nil {
			return fmt.Errorf("error writing template config: %v", err)
		}

		return nil
	}
	retrievePromptConfig := func() error {
		promptData, err := os.ReadFile(_promptConfigPath)
		if err != nil {
			return fmt.Errorf("error reading %s: %v", _promptConfigPath, err)
		}
		if err := json.Unmarshal(promptData, &lConfig); err != nil {
			return fmt.Errorf("error parsing prompt.json: %v", err)
		}
		return nil
	}

	if _, err := os.Stat(_promptConfigPath); os.IsNotExist(err) {
		// If prompt config file does not exist we will always need to create a new file.
		if err = writePromptConfig(""); err != nil {
			return nil, err
		}
		// An all new file has been created, so let's load up the config and send it.
		if err := retrievePromptConfig(); err != nil {
			return nil, err
		}

		fmt.Printf("Prompt config has been populated using intial default values\n - file location: %s\n", _promptConfigPath)
	} else {
		// Config already exists, so load from existing file
		// (we'll validate a few of the key properties at the last step, as needed).
		if err := retrievePromptConfig(); err != nil {
			return nil, err
		}

		if !newInitOnly {
			// This is NOT a new init op, so let's do a full refresh.
			// First, re-write the default config back to the file while keeping the API Key.
			if err := writePromptConfig(lConfig.APIKey); err != nil {
				return nil, err
			}
			// Now, we can reload the config from the file we just reset to defaults.
			if err := retrievePromptConfig(); err != nil {
				return nil, err
			}

			fmt.Printf("Prompt config has been reset using intial default values\n - file location: %s\n", _promptConfigPath)
		}
	}

	if newInitOnly {
		// This is only necessary in this case since the returned config is for and Image gen request.
		// Check for API key
		if lConfig.APIKey == "" || lConfig.APIKey == "YOUR_API_KEY" {
			return nil, fmt.Errorf("no API key found in config file %s", _promptConfigPath)
		}
		// Set defaults if not specified
		if lConfig.Width <= 0 {
			lConfig.Width = 1280
		}
		if lConfig.Height <= 0 {
			lConfig.Height = 1280
		}
		if lConfig.Steps <= 5 {
			lConfig.Steps = 5
		}
		if lConfig.Steps > 50 {
			lConfig.Steps = 50
		}
	}

	return &lConfig, nil
}

// ### CONFIG HANDLERS ###
// ========================================================================
func InitializeVeniceDemoConfig() error {
	// Attempt to create elements template using initial demo values.
	if err := SetDefaultElementsConfig(true); err != nil {
		return err
	}

	if _, err := initPromptConfig(false); err != nil {
		return err
	}

	return nil
}

func InitializeVeniceConfig() (*PromptConfig, error) {
	// Create template elements.json if it doesn't exist
	elementsPath := filepath.Join(VeniceDir, "elements.json")
	if _, err := os.Stat(elementsPath); os.IsNotExist(err) {
		// Attempt to create elements template using initial demo values.
		if err := SetDefaultElementsConfig(true); err != nil {
			return nil, fmt.Errorf("ERROR: %v", err)
		}
	}

	config, err := initPromptConfig(true)
	if err != nil {
		return nil, fmt.Errorf("ERROR: %v", err)
	}
	return config, nil
}
