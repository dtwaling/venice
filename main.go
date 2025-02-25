package main

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"venice/img_gen"

	"github.com/dixonwille/wmenu/v5"
	"gopkg.in/workanator/go-ataman.v1"
)

const (
	// Primary menu items
	mi  = "{light_green+bold}%s{-}"
	mi2 = "{light_green+bold}%s {-}{intensive_red+bold}-{-} {cyan}%s{-}"

	// Secondary menu items
	mi3 = "{intensive_cyan}%s{-}"
	mi4 = "{intensive_cyan}%s {-}{intensive_red+bold}-{-} {blue}%s{-}"

	// Tertiary menu items
	mi5 = "{intensive_yellow+bold}%s{-}"
	mi6 = "{yellow+bold}%s - {-}{intensive_yellow}%s{-}"

	// Quaternary menu items
	mi7 = "{red}%s{-}"
	mi8 = "{intensive_red}%s {-}{intensive_blue}-{-} {red}%s{-}"

	warn  = "{red+bold}❌{-} {intensive_red}%s{-} {red+bold}❌{-}"
	title = "{intensive_magenta+bold+underline}%s{-}"
)

var _rndr = ataman.NewRenderer(ataman.CurlyStyle())
var _doContinue bool
var _currentUser *user.User
var _veniceDir string

// ### HELPER METHODS ###
// ========================================================================
func rndr_mi(s1, s2 string, menuLvl int) string {
	if menuLvl == 1 {
		if s2 == "" {
			return fmt.Sprint(_rndr.MustRenderf(mi, s1))
		} else {
			return fmt.Sprint(_rndr.MustRenderf(mi2, s1, s2))
		}
	}

	if menuLvl == 2 {
		if s2 == "" {
			return fmt.Sprint(_rndr.MustRenderf(mi3, s1))
		} else {
			return fmt.Sprint(_rndr.MustRenderf(mi4, s1, s2))
		}
	}

	if menuLvl == 3 {
		if s2 == "" {
			return fmt.Sprint(_rndr.MustRenderf(mi5, s1))
		} else {
			return fmt.Sprint(_rndr.MustRenderf(mi6, s1, s2))
		}
	}

	if menuLvl == 4 {
		if s2 == "" {
			return fmt.Sprint(_rndr.MustRenderf(mi7, s1))
		} else {
			return fmt.Sprint(_rndr.MustRenderf(mi8, s1, s2))
		}
	}

	return ""
}

func mOptNotImplemented(opt wmenu.Opt) error {
	fmt.Println()
	fmt.Println(_rndr.MustRenderf(warn, "Not yet implemented."))
	fmt.Println()
	return nil
}

func getMainMenu() *wmenu.Menu {
	optFuncImgGen := func(opt wmenu.Opt) error {
		fmt.Println()
		fmt.Println("Starting Image generation.")
		fmt.Println()
		img_gen.CurrentUser = _currentUser
		img_gen.VeniceDir = _veniceDir
		img_gen.Req_ImgGen()
		return nil
	}
	optFuncResetElements := func(opt wmenu.Opt) error {
		fmt.Println()
		fmt.Println("Resetting image generation elements config to defaults.")
		img_gen.SetDefaultElementsConfig(false)
		fmt.Println()
		return nil
	}
	optFuncResetImgGenConfigs := func(opt wmenu.Opt) error {
		fmt.Println()
		fmt.Println("Resetting image generation configs to default values.")
		fmt.Println()
		img_gen.CurrentUser = _currentUser
		img_gen.VeniceDir = _veniceDir
		if err := img_gen.InitializeVeniceDemoConfig(); err != nil {
			fmt.Println(_rndr.MustRenderf(warn, fmt.Sprint("ERROR: ", err)))
		}
		fmt.Println()
		return nil
	}
	optFuncUpdateStylesElement := func(opt wmenu.Opt) error {
		fmt.Println()
		img_gen.VeniceDir = _veniceDir
		if err := img_gen.UpdateDefaultStylesElement(); err != nil {
			fmt.Println(_rndr.MustRenderf(warn, fmt.Sprint("ERROR: ", err)))
		}
		fmt.Println()
		return nil
	}
	optFuncExit := func(opt wmenu.Opt) error {
		fmt.Println()
		fmt.Println("Exiting Venice CLI.")
		fmt.Println()
		_doContinue = false
		return nil
	}

	menu := wmenu.NewMenu(_rndr.MustRenderf(title, "Choose an action below:"))
	menu.Option(rndr_mi("Generate Images (via prompt.json config)", "", 1), nil, false, optFuncImgGen)
	menu.Option(rndr_mi("Image Gen", "download image styles list (updates default_elements.json)", 1), nil, false, optFuncUpdateStylesElement)
	menu.Option(rndr_mi("Image Gen", "download available AI models list", 1), nil, false, mOptNotImplemented)
	menu.Option(rndr_mi("Image Gen", "reset Elements config to defaults", 2), nil, false, optFuncResetElements)
	menu.Option(rndr_mi("Image Gen", "reset both configs to defaults", 2), nil, false, optFuncResetImgGenConfigs)
	menu.Option(rndr_mi("Upscale Image", "", 4), nil, false, mOptNotImplemented)
	menu.Option(rndr_mi("Exit", "", 3), nil, true, optFuncExit)

	return menu
}

func setGlobalVars() bool {
	// Get current user's home directory
	currentUser, err := user.Current()
	if err != nil {
		fmt.Println(_rndr.MustRenderf(warn, fmt.Sprint("Error getting current user: ", err)))
		return false
	}
	_currentUser = currentUser

	// Create .venice directory if it doesn't exist
	_veniceDir = filepath.Join(currentUser.HomeDir, ".venice")
	if err := os.MkdirAll(_veniceDir, 0755); err != nil {
		fmt.Println(_rndr.MustRenderf(warn, fmt.Sprint("Error creating .venice directory: ", err)))
		return false
	}
	return true
}

// ### YOUR TICKET TO THE MEAT n POTATAHS ###
// ========================================================================
func main() {
	_doContinue = setGlobalVars()

	if _doContinue {
		menu := getMainMenu()
		for _doContinue {
			err := menu.Run()
			if err != nil {
				fmt.Println()
				fmt.Println(_rndr.MustRenderf(warn, fmt.Sprint("ERROR: ", err)))
				fmt.Println("Please select a valid option!")
				fmt.Println()
			}
		}
	}
}
