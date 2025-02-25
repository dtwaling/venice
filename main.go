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
	mi    = "{light_green+bold}%s{-}"
	mn    = "{light_green+bold}%s ({-}{light_blue}%s{-}{light_green+bold}){-}"
	warn  = "{red+bold}❌{-} {light_red}%s{-} {red+bold}❌{-}"
	title = "{intensive_magenta+bold+underline}%s{-}"
)

var _rndr = ataman.NewRenderer(ataman.CurlyStyle())
var _doContinue bool
var _currentUser *user.User
var _veniceDir string

// ### HELPER METHODS ###
// ========================================================================
func rndr_mi(s1, s2 string) string {
	if s2 == "" {
		return fmt.Sprint(_rndr.MustRenderf(mi, s1))
	} else {
		return fmt.Sprint(_rndr.MustRenderf(mn, s1, s2))
	}
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
		fmt.Println("Resetting Elements config to defaults.")
		img_gen.SetDefaultElementsConfig(false)
		fmt.Println()
		return nil
	}
	optFuncResetDemoValues := func(opt wmenu.Opt) error {
		fmt.Println()
		fmt.Println("Resetting configs to demo values.")
		fmt.Println()
		img_gen.CurrentUser = _currentUser
		img_gen.VeniceDir = _veniceDir
		if err := img_gen.InitializeVeniceDemoConfig(); err != nil {
			fmt.Println(_rndr.MustRenderf(warn, fmt.Sprint("ERROR: ", err)))
		}
		//img_gen.SetDefaultElementsConfig(true)
		//fmt.Println(rndr.MustRenderf(warn, "Not yet fully implemented."))
		fmt.Println()
		return nil
	}
	optFuncNotImpl := func(opt wmenu.Opt) error {
		fmt.Println()
		fmt.Println(_rndr.MustRenderf(warn, "Not yet implemented."))
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
	menu.Option(rndr_mi("Generate Images", "via prompt.json"), nil, false, optFuncImgGen)
	menu.Option(rndr_mi("Upscale Image", ""), nil, false, optFuncNotImpl)
	menu.Option(rndr_mi("Refresh and update image styles", "updates default_elements.json"), nil, false, optFuncNotImpl)
	menu.Option(rndr_mi("Reset Elements config to defaults", ""), nil, false, optFuncResetElements)
	menu.Option(rndr_mi("Reset configs to initialDemo values", ""), nil, false, optFuncResetDemoValues)
	menu.Option(rndr_mi("Exit", ""), nil, false, optFuncExit)

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
