# Venice AI Image Generator

Venice is a command-line tool for generating AI images using the Venice.ai API. It provides extensive customization options and manages image generation queues automatically.

## Author

[Original work by Hunter Rose (@HunterR0se)](https://twitter.com/HunterR0se)

## Installation

1. Create a .venice directory in your home folder:

```bash
mkdir ~/.venice
```

2. Two configuration files are needed in the ~/.venice directory:
    - prompt.json (configuration settings)
    - elements.json (customization elements)

The application will create template versions of these files if they don't exist.

## Configuration

### API Key

Go to [Visit Venice.AI](https://venice.ai) and select "API" to generate an API key. **Only available to paying members**.

You'll need to add your Venice.ai API key to prompt.json.

If prompt.json does not exist, you will be prompted to provide your API key in the terminal, then a new file will be generated with the pre-sets below:

```json
{
    "model": "fluently-xl",
    "prompt_name": "Hooded Hacker",
    "name_as_subdir": true,
    "prompt": "a modern hacker wearing a hoodie",
    "negative_prompt": "blur, distort, distorted, blurry, censored, censor, pixelated",
    "output_dir": "/home/YOURHOME/Pictures/venice",
    "api_key": "YOUR_API_KEY",
    "num_images": 42,
    "min_config": 7.5,
    "max_config": 15.0,
    "height": 1280,
    "width": 1280,
    "steps": 35,
    "style": true,
    "enable_face": true,
    "enable_type": true,
    "enable_clothing": true,
    "enable_poses": true,
}
```

### Key Settings

- `Model`: Available models include:

    - fluently-xl (default, fastest)
    - flux-dev (highest quality)
    - flux-dev-uncensored
    - pony-realism (most uncensored)
    - lustify-sdxl (too sexy 4 u ..or maybe gross ...probably)
    - stable-diffusion-3.5 (most creative - supposedly)

- `NumImages`: How many images to generate
- `Width/Height`: Image dimensions (default 1280x1280)
- `Steps`: Generation steps (5-50, default 35)
- `OutputDir`: Where generated images are saved

### Feature Toggles

Enable/disable specific enhancement categories:

```json
{
    "EnableFace": true,
    "EnableType": false,
    "EnableHair": false,
    "EnableEyes": false,
    "EnableClothing": true,
    "EnableBackground": false,
    "EnablePoses": false,
    "EnableAccessories": false,
    "EnableDirty": false
}
```

## Output

- Generated images are saved to the specified OutputDir (default: ~/Pictures/venice).
    - If name-as-subdir is true, a new subfolder is created for each run in the main output folder.
- Filenames include the image iteration, seed, cfg scale.
- Promt enhancements are recorded for each image in a PromptLog.txt file.  Each entry records filename, image style, and added elements.
- Progress display shows:
    - Completion percentage
    - Current status
    - Active prompt
    - Model & configuration
    - Feature toggle states
    - Error status

## Rate Limiting

The application automatically handles rate limiting:

- 2 second delay between generations
- Automatic retries on errors
- Graceful handling of API limits

## Error Handling

- Failed generations are tracked and displayed
- Error messages appear in the progress display and get recorded to the PromptLog.txt file
- Auto-retry for common errors (rate limits, server issues)
    - Also attempts to mitigates "bad request" spamming in the event if errors caused by something in a prompt.

## Usage

1. Ensure your API key is set in ~/.venice/prompt.json
2. Run the application:

```bash
./venice
```

3. Monitor progress in the terminal display
4. Use Ctrl+C to gracefully stop generation

## Customization

The elements.json file contains categorized prompt elements for:

- Styles
- Face features
- Character types
- Hair styles
- Eye colors
- Clothing
- Poses
- Accessories
- Backgrounds

Edit these categories to customize the available elements for generation.

## Tips

- Keep prompts under 1250 characters
- Monitor the error display for issues
- Use Ctrl+C for clean shutdown
- Check output directory for generated images
- Adjust cfg_scale (7.5-15.0) to control generation stability

## Requirements

- Venice.ai API key
- Write access to ~/.venice directory
- Sufficient disk space for output

```

```
