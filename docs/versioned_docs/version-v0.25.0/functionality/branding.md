---
title: Branding
---

# Branding

Branding allows administrators to customize the visual appearance of the Obot platform. Access this page from **Branding** in the sidebar.

## Theme

Select **Theme** in the configuration sidebar to customize colors and typography.

### Mode (Light / Dark)

Use **Mode** to choose which color scheme you are editing. The preview updates immediately so you can tune light and dark appearances separately.

### Surfaces (Custom / Tinted)

Under **Surfaces**, choose how page and panel backgrounds are set:

#### Custom

- **Background**: Page background
- **Surface 1, 2, 3**: Layered UI surfaces (cards, panels, nested regions)

Use the color swatch or enter a value in the text field. Supported formats include hex (`#rrggbb`), `hsl()` / `hsla()`, and `oklch()`.

#### Tinted

Derive surfaces from Obot’s built-in default surface ramp using three sliders for the active mode:

- **Hue**: Shifts the overall surface hue
- **Tint**: Adds color tint (0–100%)
- **Shade**: Lightens or darkens the ramp; the center position is neutral

### Per-Theme Colors

The **Per-Theme Colors** toggle controls whether accent, button/indicator, and text colors are editable separately. This is beneficial if you need to make individual adjustments to the colors below per theme.

### Accent Color

The primary brand color used for primary buttons, links, highlights, and similar emphasis.

### Buttons & Indicators

Colors for non-primary actions and status styling in the active mode:

- **Secondary**: Secondary buttons (for example **Cancel**)
- **Success**: Success states and success buttons
- **Warning**: Warning states and warning buttons
- **Error**: Error states and error buttons

### Text

Typography and text-on-color settings for the active mode:

- **Base Font Color**: Default body and UI text
- **On-Accent Button Text**: Text on primary (accent) buttons
- **Success Button Text**: Text on success buttons
- **Warning Button Text**: Text on warning buttons
- **Error Button Text**: Text on error buttons
- **Font Family**: UI font stack—**Poppins** (default), **Helvetica Neue**, or **System Default**

## Logos

Select **Logos** in the configuration sidebar to replace icons and full logos.

### Icons

These apply across light and dark mode:

- **Default Icon**: The standard icon shown in the UI
- **Error Icon**: Icon displayed for error states
- **Warning Icon**: Icon displayed for warning states

### Full Logos

Full logos depend on the selected **Mode** (light or dark):

- **Full Logo**: The main logo shown in the navbar (separate versions for light and dark schemes)
- **Full Enterprise Logo**: Logo used when running enterprise version of Obot
- **Full Chat Logo**: Logo used for agent chatting in Obot

Changes preview live in the main area; they are not stored until you click **Save**.

## Restoring Defaults

Click **Restore Default** to reset all preferences to the original Obot theme.