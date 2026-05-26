# WP Engine Go CLI

A premium, interactive Go command line tool designed to manage WP Engine environments, spin up new installs, and automate WordPress core/plugin/theme updates safely. 

This CLI features an automated backup system—polling the WP Engine API until a checkpoint is complete—before connecting via SSH to execute `wp-cli` commands. The terminal interface is styled using Charm's `lipgloss` and `bubbletea` for a visually stunning and responsive user experience.

---

## Key Features

- **Automated Backup Assurance**: Every update is preceded by an API-triggered backup checkpoint, verified via real-time status polling.
- **Selective & Batch Updates**: Run updates on single environments or batches via CSV/files or all active environments. Support for `--plugins`, `--themes`, `--core`, and `--dry-run`.
- **Interactive Dashboard**: A Bubble Tea TUI displaying active progress bars, spinners, and live logs.
- **Non-Interactive Mode**: Outputs clean, colorized Lipgloss logs (`--no-interactive`) suited for automation and CI/CD pipelines.
- **SSH Agent Support**: Connects securely to the WP Engine SSH Gateway using SSH key files or local SSH agents.
- **Local Environment Cache**: Resolves target environment UUIDs instantly using a local configuration cache (`~/.wpengine-cli-cache.json`) built automatically when listing sites or environments.

---

## Prerequisites

1. **Go 1.20+** installed on your system.
2. **WP Engine API Access**:
   - Log in to the [WP Engine User Portal](https://my.wpengine.com/).
   - Navigate to **Users > API Access**.
   - Generate API Credentials (you will get a username and password).
3. **SSH Key Setup**:
   - Make sure your public SSH key is added to your user profile in the WP Engine User Portal.
   - The CLI will authenticate using your SSH private key (defaults to `~/.ssh/id_ed25519` or `~/.ssh/id_rsa`).

---

## Setting up SSH Keys on WP Engine

To allow the CLI to execute updates via SSH, you must add your public SSH key to your WP Engine profile.

### Step 1: Generate an SSH Key Pair (if you don't have one)
In your terminal (PowerShell, Command Prompt, or Bash), run:
```bash
ssh-keygen -t ed25519 -C "your_email@example.com"
```
Press Enter to accept the default file location (`~/.ssh/id_ed25519`). You can optionally specify a passphrase.

### Step 2: Retrieve your Public Key
Copy the contents of the generated `.pub` file:
* **Windows (PowerShell)**: `Get-Content ~\.ssh\id_ed25519.pub`
* **macOS / Linux**: `cat ~/.ssh/id_ed25519.pub`

### Step 3: Add to the WP Engine Portal
1. Log in to the [WP Engine User Portal](https://my.wpengine.com).
2. Click your **Profile Icon** in the top right corner.
3. Select **Profile & Settings**.
4. In the left navigation menu, click **SSH Keys** (or navigate directly to [https://my.wpengine.com/profile/ssh_keys](https://my.wpengine.com/profile/ssh_keys)).
5. Click **New SSH Key** or **Add SSH Key**.
6. Paste the copied public key, name it (e.g., "Dev Laptop"), and click **Add**.

*Note: The key can take up to 5-10 minutes to propagate to all environment gateways.*

---

## Installation & Setup

1. **Build the CLI**
   Navigate to the project root directory and build the binary:
   ```bash
   go build -o wpengine
   ```
   *(On Windows, this creates `wpengine.exe`)*

2. **Add to PATH (Optional)**
   To run the `wpengine` command from any directory, add it to your system's `PATH`:

   * **macOS / Linux**:
     Create a symbolic link to the binary in a directory that is already in your `PATH` (like `/usr/local/bin`):
     ```bash
     sudo ln -sf "$(pwd)/wpengine" /usr/local/bin/wpengine
     ```

   * **Windows (PowerShell)**:
     Add the directory containing `wpengine.exe` to your User PATH variable:
     ```powershell
     [System.Environment]::SetEnvironmentVariable(
         "Path",
         [System.Environment]::GetEnvironmentVariable("Path", "User") + ";$((Get-Item .).FullName)",
         "User"
     )
     ```
     *(Note: Restart your terminal after running this command for the changes to take effect.)*

3. **Configure your Credentials**
   Set up your WP Engine API credentials and default account details:
   ```bash
   ./wpengine config set --username <api_username> --password <api_password> --account-id <default_account_uuid>
   ```

   *Optional flags for `config set`:*
   - `--ssh-key-path <path>`: Specify a custom SSH private key (default is autodetected).
   - `--ssh-passphrase <passphrase>`: Set a passphrase if your key is encrypted.
   - `--batch-concurrency <num>`: Set default maximum parallel update workers (default: `10`).
   - `--interactive <true|false>`: Enable/disable the interactive TUI by default.

4. **Verify Configuration**
   Confirm that your configuration is saved (sensitive values are masked):
   ```bash
   ./wpengine config show
   ```

5. **Initialize the Environment Cache**
   To speed up updates and checks, initialize your local environment cache by listing your sites:
   ```bash
   ./wpengine site list
   ```
   *(This fetches all sites and environments across your accounts and saves them to `~/.wpengine-cli-cache.json` for rapid, zero-API environment name resolution.)*

---

## Usage Guide

### 1. View Accounts
List all WP Engine accounts you have access to, displaying their UUIDs for easy copying:
```bash
./wpengine account list
```

### 2. View Environments (Installs)
List all environments linked to your account in a beautifully formatted table:
```bash
./wpengine env list
```

You can also filter the environments by type using flags:
* `-p, --production`: Filter to only production environments.
* `-s, --staging`: Filter to only staging environments.
* `-d, --dev`: Filter to only development environments.

Example:
```bash
./wpengine env list --production
```

### 3. View Sites
List all top-level sites under your account (including a column displaying their associated environments):
```bash
./wpengine site list
```

You can also filter the sites to show only those containing specific environment types using flags:
* `-p, --production`: Filter to sites containing a production environment.
* `-s, --staging`: Filter to sites containing a staging environment.
* `-d, --dev`: Filter to sites containing a development environment.

Example:
```bash
./wpengine site list --production --staging
```

### 4. Spin Up an Environment
Create a new environment install:
```bash
./wpengine env create --name my-dev-sandbox --type development
```

### 5. Delete an Environment
Terminate an install using its ID:
```bash
./wpengine env delete <install_uuid>
```

### 6. Check for Outstanding Updates
Check which WordPress core, plugins, or themes have updates available on a single environment:
```bash
./wpengine check my-dev-sandbox
```

Check updates across all active environments concurrently:
```bash
./wpengine check --all-envs
```

**Minimal Output Flag**
To minimize verbose output and display a summary table of the environments' update status:
```bash
./wpengine check --all-envs --minimal
# or
./wpengine check --all-envs -m
```

**Cached Results & No-Flag Default**
Every check run automatically caches its results locally to `~/.wpengine-cli-check-results.json` along with a timestamp. 
* To view the cached summary table of the last check run without hitting the API or SSH, simply run the command with no flags or arguments:
  ```bash
  ./wpengine check
  ```
* **Auto-update Sync**: When you run `./wpengine update` successfully on an environment, the local cache is automatically updated in real-time to remove the updated items, keeping your cached check results in sync.

### 7. Run Website Updates
Update a single website (triggers backup, polls progress, runs SSH update):
```bash
./wpengine update my-dev-sandbox --plugins
```

You can optionally specify one or more email addresses for WP Engine backup completion notifications using the `-e, --email` flag (if not provided, it defaults to a quiet `no-reply@wpengine.com`):
```bash
./wpengine update my-dev-sandbox --plugins --email admin@example.com
```

Run a dry-run check updating core, plugins, and themes on all active environments in parallel:
```bash
./wpengine update --all --all-envs --dry-run
```

Run updates from a batch list file (one environment name per line):
```bash
./wpengine update --plugins --batch target_envs.txt --no-interactive
```

---

## Code Architecture

- `main.go`: App entrypoint.
- `cmd/`: Command definitions using Cobra (`root`, `config`, `env`, `site`, `update`).
- `internal/api/`: WP Engine API client implementation & models.
- `internal/ssh/`: SSH client connecting to WP Engine Gateway to run `wp-cli`.
- `internal/ui/`: Bubble Tea and Lipgloss CLI components.
- `internal/config/`: Config Loader and Writer (`~/.wpengine-cli.yaml`).

---

## Disclaimer

This is a fun, unofficial hobby project and is not affiliated with, authorized, maintained, sponsored, or endorsed by WP Engine.
