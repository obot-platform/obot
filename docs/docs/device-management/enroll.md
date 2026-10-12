---
title: "Enroll devices"
---

## Configuration {#configuration}

The Configuration view sets up [Obot Sentry](https://github.com/obot-platform/obot-sentry), a lightweight agent that enrolls workstations with Obot and enables device scanning, local agent audit logs, and optional tool call enforcement on enrolled devices.

On first visit, select **Get Started** to create the device configuration, then follow the numbered install guide:

1. **Generate an enrollment key.** New devices enroll with your Obot server using this key. The credential is revealed once, when the key is created.
2. **Select your installation method.** How Obot Sentry is delivered to your devices.
3. **Select an operating system.** The operating system your devices run.
4. **Download the install artifacts.** A ZIP package for the selected installation method and operating system.
5. **Follow the install instructions.** The steps match your selections and show where to put the enrollment key.

Once Obot Sentry is installed and enrolled, devices begin reporting to Obot and appear in the other Device Management views.

### Installation methods {#installation-methods}

Installation methods and operating systems vary by Obot Sentry release. The current release supports:

| Installation method | Operating system | What the download contains |
|---------------------|------------------|----------------------------|
| Do it Yourself | Windows | An MSI that sets up automatic scanning and audit hook maintenance, and handles upgrades and uninstalls. A standalone `obot-sentry.exe` is also included for running one-off scans or installing hooks manually. |
| Do it Yourself | macOS | A standalone `obot-sentry` binary for running scans and installing audit hooks manually. |
| Microsoft Intune | Windows | An `.intunewin` package to deploy as a Windows app (Win32) from the Intune admin center. Assigned device groups install Obot Sentry on their next check-in, then scan and maintain audit hooks automatically. |

Each download's install instructions cover installing, configuring the enrollment key, and uninstalling for that method and operating system.

### Enrollment keys {#enrollment-keys}

- Keys can be named and given an expiration date. The default expiration is one year.
- Revoking a key stops new devices from enrolling with it. Already-enrolled devices are unaffected.
- New devices cannot enroll until at least one key exists.

### Settings and updates {#settings-and-updates}

Open the agent settings (the gear icon) to review or change how Obot Sentry behaves on your devices.

The available settings also vary by Obot Sentry release. The current release has one:

| Setting | Description | Default |
|---------|-------------|---------|
| Scan interval (minutes) | How often each signed-in user's device submits a scan. Accepts 15–1440 minutes. | 60 |

Use **Check for updates** to pick up new Obot Sentry releases. If an Update available badge appears, save the agent settings to rebuild your downloads with the new release.
