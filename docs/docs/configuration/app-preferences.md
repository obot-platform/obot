# App Preferences

The App Preferences page in the Admin interface allows administrators to customize the visual appearance and branding of the obot platform. These settings affect all users and provide a consistent branded experience across the application.

## Accessing App Preferences

Navigate to **Admin > App Preferences** in the main navigation to access the configuration page.

## Theme Colors

Customize the color scheme for both light and dark modes. The theme system supports separate color palettes for each mode, allowing your branding to look great in any viewing preference.

### Light Scheme Colors

- **Primary**: The main accent color used for buttons, links, and interactive elements
- **Background**: The base background color for the application
- **Primary Text**: The main text color for content
- **Secondary Text**: Muted text color for less prominent content
- **Surface 1/2/3**: Layered surface colors for cards, panels, and UI elements

### Dark Scheme Colors

The same color options are available for dark mode, prefixed with "dark" in the API:
- `darkPrimaryColor`
- `darkBackgroundColor`
- `darkOnBackgroundColor`
- `darkOnSurfaceColor`
- `darkSurface1Color`, `darkSurface2Color`, `darkSurface3Color`

### Live Preview Mode

Enable **Live Preview Mode** to see theme color changes in real-time before saving. This allows you to experiment with colors and see how they look across the interface.

## Icons & Logos

Upload custom icons and logos to replace the default obot branding. The system supports separate logos for light and dark modes to ensure visibility in both themes.

### Standard Icons

- **Default Icon**: The main application icon used throughout the interface
- **Error Icon**: Displayed when errors occur
- **Warning Icon**: Displayed for warning states

### Theme-Specific Logos

Configure different logos for light and dark modes:

**Light Scheme:**
- Full Logo
- Full Enterprise Logo
- Full Chat Logo

**Dark Scheme:**
- Full Logo (Dark)
- Full Enterprise Logo (Dark)
- Full Chat Logo (Dark)

### Uploading Images

Click on any icon or logo to open the upload dialog. You can either:
- Upload an image file directly
- Provide a URL to an externally hosted image

Supported formats include SVG (recommended for logos), PNG, and JPEG.

## Footer Branding

Customize the footer message that appears at the bottom of chat conversations. This is useful for displaying disclaimers, product names, and support links.

### Configuration Options

| Field | Description | Default |
|-------|-------------|---------|
| **Product Name** | The name displayed in the footer message | `Obot` |
| **Issue Report URL** | URL for the "Report issues here" link | GitHub issues page |
| **Footer Message** | The disclaimer text shown to users | `{productName} isn't perfect. Double check its work.` |
| **Show Footer** | Toggle to show/hide the footer entirely | `true` |

### Using Placeholders

The footer message supports the `{productName}` placeholder, which will be replaced with the configured Product Name. This allows you to create dynamic messages like:

```
{productName} may make mistakes. Please verify important information.
```

Which would render as:

```
Obot may make mistakes. Please verify important information.
```

### Example Use Cases

**Enterprise Deployment:**
- Product Name: `Acme AI Assistant`
- Footer Message: `{productName} is provided for informational purposes only.`
- Issue Report URL: `https://support.acme.com/ai-assistant`

**Internal Tool:**
- Product Name: `Internal Helper Bot`
- Footer Message: `{productName} responses should be reviewed by qualified staff.`
- Show Footer: `true`

**Clean Interface:**
- Show Footer: `false` (hides the footer completely)

## API Reference

App Preferences can also be configured via the API:

### Get Current Preferences

```
GET /api/admin/app-preferences
```

### Update Preferences

```
PUT /api/admin/app-preferences
Content-Type: application/json

{
  "logos": {
    "logoIcon": "/user/images/custom-icon.svg",
    "logoDefault": "/user/images/custom-logo.svg",
    ...
  },
  "theme": {
    "primaryColor": "#4f7ef3",
    "backgroundColor": "hsl(0 0 100)",
    ...
  },
  "branding": {
    "productName": "My Custom Bot",
    "issueReportUrl": "https://example.com/issues",
    "footerMessage": "{productName} is here to help!",
    "showFooter": true
  }
}
```

### Branding Object Schema

| Property | Type | Description |
|----------|------|-------------|
| `productName` | string | Display name for the product |
| `issueReportUrl` | string | URL for issue reporting link |
| `footerMessage` | string | Footer text (supports `{productName}` placeholder) |
| `showFooter` | boolean | Whether to display the footer |

## Best Practices

1. **Use SVG logos** for crisp display at any resolution
2. **Test both light and dark modes** to ensure logos are visible
3. **Keep footer messages concise** to avoid cluttering the chat interface
4. **Use the Live Preview** feature before saving theme changes
5. **Provide a valid issue report URL** to help users report problems
6. **Consider accessibility** when choosing color contrasts

## Restore Defaults

Click the **Restore Default** button at the top of the App Preferences page to reset all settings to their original values. This affects logos, theme colors, and branding settings.
