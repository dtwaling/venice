package img_gen

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	GETSTYLES_URI = "https://api.venice.ai/api/v1/image/styles"
)

type PromptElements struct {
	// Image styles - change the overall artistic style of the image
	Style []string `json:"style"`
	// Base attributes
	Face     []string `json:"face"`
	Type     []string `json:"type"`
	Hair     []string `json:"hair"`
	Eyes     []string `json:"eyes"`
	Clothing []string `json:"clothing"`

	// Extra elements
	Poses       []string `json:"poses"`
	Accessories []string `json:"accessories"`
	Backgrounds []string `json:"backgrounds"`

	// Keep custom the same
	Custom []string `json:"custom"`
}

// ## - PromptElements extension methods - ##
func (tmplt *PromptElements) GetDefaultElementsConfig(isInit bool) {
	if isInit {
		tmplt.Style = E_STYLE_DEMO.LMNTItems
		tmplt.Backgrounds = E_BKGRND_DEMO.LMNTItems
		tmplt.Custom = E_CUST_DEMO.LMNTItems
	} else {
		tmplt.Style = E_STYLE.LMNTItems
		tmplt.Backgrounds = E_BACKGROUNDS.LMNTItems
		tmplt.Custom = E_CUSTOM.LMNTItems
	}

	tmplt.Face = E_FACE.LMNTItems
	tmplt.Type = E_TYPE.LMNTItems
	tmplt.Hair = E_HAIR.LMNTItems
	tmplt.Eyes = E_EYES.LMNTItems
	tmplt.Clothing = E_CLOTHING.LMNTItems
	tmplt.Poses = E_POSES.LMNTItems
	tmplt.Accessories = E_ACCESSORIES.LMNTItems
}

type ElementCategory struct {
	LMNTName  string
	LMNTItems []string
}
type DefaultElements struct {
	Elements []ElementCategory
}

// ### CONST (but not really const) ###
// ========================================================================
var E_STYLE = ElementCategory{
	"style",
	[]string{
		"3D Model", "Analog Film", "Anime", "Cinematic", "Fantasy Art",
		"Line Art", "Neon Punk", "Origami", "Photographic", "Pixel Art",
		"Texture", "Abstract", "Cubist", "Graffiti", "Hyperrealism",
		"Impressionist", "Renaissance", "Steampunk", "Surrealist", "Typography",
		"Watercolor", "Fighting Game", "Super Mario", "Minecraft", "Pokemon",
		"Retro Arcade", "Retro Game", "RPG Fantasy Game", "Strategy Game",
		"Street Fighter", "Legend of Zelda", "Dreamscape", "Dystopian",
		"Fairy Tale", "Gothic", "Grunge", "Horror", "Minimalist", "Monochrome",
		"Space", "Techwear Fashion", "Tribal", "Alien", "Film Noir", "HDR",
		"Long Exposure", "Neon Noir", "Silhouette", "Tilt-Shift"}}
var E_STYLE_DEMO = ElementCategory{
	"style",
	[]string{
		"3D Model", "Analog Film", "Anime", "Cinematic",
		"Line Art", "Neon Punk", "Origami", "Photographic",
		"Graffiti", "Hyperrealism", "Renaissance",
		"Steampunk", "Watercolor", "RPG Fantasy Game",
		"Dreamscape", "Dystopian", "Gothic", "Grunge",
		"Minimalist", "Monochrome", "Space", "Alien", "HDR",
		"Long Exposure", "Neon Noir", "Tilt-Shift"}}
var E_FACE = ElementCategory{
	"face",
	[]string{
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
		"dot patterns", "triangle designs"}}
var E_TYPE = ElementCategory{
	"type",
	[]string{
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
		"raven feather pattern", "phoenix feather arrangement"}}
var E_HAIR = ElementCategory{
	"hair",
	[]string{
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
		"gothic curly look", "short copper strands"}}
var E_EYES = ElementCategory{
	"eyes",
	[]string{
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
		"gazelle-like eyes", "lynx-inspired gaze"}}
var E_CLOTHING = ElementCategory{
	"clothing",
	[]string{
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
		"mage robe-style costume"}}
var E_POSES = ElementCategory{
	"poses",
	[]string{
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
		"heroic and confident", "mysterious and intriguing"}}
var E_ACCESSORIES = ElementCategory{
	"accessories",
	[]string{
		"statement piece of jewelry", "designer bag", "fashionable sunglasses",
		"elegant watch", "simple yet elegant necklace",
		"bold and colorful scarf", "stylish hat", "luxurious fur coat",
		"vintage-inspired brooch", "eclectic bohemian accessories",
		"understated minimalist accessories", "elegant gloves",
		"luxurious decadent accessories", "whimsical playful items",
		"heroic confident gear", "romantic sentimental trinkets",
		"mysterious intriguing adornments",
		"vibrant energetic decor", "peaceful serene pieces",
		"adventurous bold items", "sophisticated elegant goods"}}
var E_BACKGROUNDS = ElementCategory{
	"backgrounds",
	[]string{
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
		"crystal cave interior setting", "vintage train station backdrop"}}
var E_BKGRND_DEMO = ElementCategory{
	"backgrounds",
	[]string{
		"sunny beach setting", "bustling city street", "serene mountain vista",
		"peaceful forest landscape", "eclectic bohemian-inspired locale",
		"sophisticated refined environment", "understated minimalist space",
		"heroic confident stadium", "mysterious intriguing abandoned site",
		"peaceful serene lakeside", "adventurous bold wilderness",
		"heroic confident skyscraper backdrop", "romantic sentimental bridge view",
		"mysterious intriguing foggy alleyway", "distressed brick wall with colorful graffiti",
		"gritty industrial warehouse scene", "neon-lit cyberpunk street environment",
		"overgrown urban ruins setting", "desert oasis with palm trees backdrop",
		"snowy mountain peak vista", "ancient stone temple setting",
		"futuristic space station view", "seaside boardwalk at sunset",
		"bustling open-air market area", "historic cobblestone street vista",
		"foggy lighthouse cliff view", "autumn forest path",
		"crystal cave interior setting"}}
var E_CUSTOM = ElementCategory{"custom", []string{}}
var E_CUST_DEMO = ElementCategory{
	"custom",
	[]string{
		"Chevy Corvettes", "Chevy Camaro",
		"Ford Mustang", "Dodge Viper", "Dodge Charger",
		"Ford Pinto", "1930s jalopy", "1890s steam-powered",
		"Porsche 911", "Ferrari", "Maserati",
		"Model T Roadster", "1938 Buick", "jet-fueled funny-car",
		"Ford Thunderbird", "Belle Air", "hover-cars",
		"1970s street-rod", "Muscle-car vs Super-car"}}

// ### ELEMENTS HELPERS ###
// ========================================================================
func GetImageStylesAPIResponse(apiKey string) ([]byte, error) {
	for i := range 3 {
		req, _ := http.NewRequest("GET", GETSTYLES_URI, nil)
		req.Header.Add("Authorization", "Bearer "+apiKey)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Printf("Failed attempt %d of 3", i+1)
			fmt.Println("Error retrieving response: ", err)
			i++
			time.Sleep(10 * time.Second)
			continue
		}
		defer resp.Body.Close()

		rBody, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Printf("Failed attempt %d of 3", i+1)
			fmt.Println("Error reading response body: ", err)
			i++
			time.Sleep(10 * time.Second)
			continue
		}

		return rBody, nil
	}

	return nil, fmt.Errorf("Failed to retrieve updated image styles list.")
}

// ### ELEMENTS HANDLERS ###
// ========================================================================
func SetDefaultElementsConfig(useDemoValues bool) error {
	// Set template paths for default_elements.json and elements.json
	userElementsPath := filepath.Join(VeniceDir, "elements.json")
	defElementsPath := filepath.Join(VeniceDir, "default_elements.json")
	var templateElements PromptElements

	// Create default_elements.json if it doesn't exist.
	if _, err := os.Stat(defElementsPath); os.IsNotExist(err) {
		templateElements.GetDefaultElementsConfig(false)
		elementJSON, err := json.MarshalIndent(templateElements, "", "    ")
		if err != nil {
			return fmt.Errorf("error creating template default elements: %v", err)
		}
		if err := os.WriteFile(defElementsPath, elementJSON, 0644); err != nil {
			return fmt.Errorf("error writing template default elements: %v", err)
		}

		fmt.Printf("Created default elements template at %s\n", defElementsPath)
	}

	if !useDemoValues {
		defFile, err := os.Open(defElementsPath)
		if err != nil {
			return fmt.Errorf("error opening default elements file: %v", err)
		}
		defer defFile.Close()

		userFile, err := os.OpenFile(userElementsPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return fmt.Errorf("error opening user elements file: %v", err)
		}
		defer userFile.Close()

		_, err = io.Copy(userFile, defFile)
		if err != nil {
			return fmt.Errorf("error copying default elements to user elements: %v", err)
		}

		fmt.Printf("User elements config has been populated using full set of default values.\n - file location: %s\n", userElementsPath)
	} else {
		// Attempt to create elements template using initial demo values.
		templateElements.GetDefaultElementsConfig(true)
		elementJSON, err := json.MarshalIndent(templateElements, "", "    ")
		if err != nil {
			return fmt.Errorf("error creating user elements config: %v", err)
		}
		if err := os.WriteFile(userElementsPath, elementJSON, 0644); err != nil {
			return fmt.Errorf("error writing user elements config: %v", err)
		}

		fmt.Printf("User elements config has been populated using initial demo values.\n - file location: %s\n", userElementsPath)
	}

	return nil
}

func UpdateDefaultStylesElement() error {
	config, err := InitializeVeniceConfig()
	if err != nil {
		return fmt.Errorf("error retrieving Venice config: %v", err)
	}

	rBody, err := GetImageStylesAPIResponse(config.APIKey)
	if err != nil {
		return fmt.Errorf("error retrieving image styles: %v", err)
	}

	var rStyles struct {
		Obj  string   `json:"object"`
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(rBody, &rStyles); err != nil {
		return fmt.Errorf("error parsing image styles from API response: %v", err)
	}

	defElementsPath := filepath.Join(VeniceDir, "default_elements.json")
	var templateElements PromptElements

	templateElements.GetDefaultElementsConfig(true)
	templateElements.Style = rStyles.Data

	elementJSON, err := json.MarshalIndent(templateElements, "", "    ")
	if err != nil {
		return fmt.Errorf("error generating JSON data for updated default elements: %v", err)
	}
	if err := os.WriteFile(defElementsPath, elementJSON, 0644); err != nil {
		return fmt.Errorf("error writing updated JSON data to default elements config: %v", err)
	}

	fmt.Printf("Styles list has been updated in the default elements config\n - file location: %s\n", defElementsPath)

	return nil
}
