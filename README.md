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

The initial template will contain:

```json
{
    "Model": "fluently-xl",
    "APIKey": "YOUR_API_KEY",
    "NegativePrompt": "blur, distort, distorted, blurry, censored, censor, pixelated",
    "NumImages": 150,
    "MinConfig": 7.5,
    "MaxConfig": 15.0,
    "Height": 1280,
    "Width": 1280,
    "Steps": 35,
    "Style": false,
    "Prompt": "a modern hacker wearing a hoodie",
    "OutputDir": "/home/YOURHOME/Pictures/venice"
}
```

### Key Settings

- `Model`: Available models include:

    - fluently-xl (default, fastest)
    - flux-dev (highest quality)
    - flux-dev-uncensored
    - pony-realism (most uncensored)

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

- Generated images are saved to the specified OutputDir (default: ~/Pictures/venice)
- Filenames include the seed, cfg scale, and prompt elements used
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
- Error messages appear in the progress display
- Auto-retry for common errors (rate limits, server issues)

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
