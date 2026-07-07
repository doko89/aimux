package login

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Run dispatches login subcommands.
// Usage: aimux login chatgpt [--force]
func Run(args []string) {
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	provider := strings.ToLower(args[0])
	force := false
	for _, a := range args[1:] {
		if a == "--force" || a == "-f" {
			force = true
		}
	}

	switch provider {
	case "chatgpt", "codex":
		runChatGPTLogin(force)
	default:
		fmt.Fprintf(os.Stderr, "Unknown provider: %s\n\n", provider)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Usage: aimux login <provider>

Providers:
  chatgpt    Login via ChatGPT OAuth (for Codex models)
  codex      Alias for chatgpt

Options:
  --force, -f   Force re-login even if already authenticated`)

	authPath := AuthFilePath()
	if _, err := os.Stat(authPath); err == nil {
		fmt.Printf("\nCurrent auth: %s\n", authPath)
	} else {
		fmt.Printf("\nNo auth found. Run: aimux login chatgpt\n")
	}
}

func runChatGPTLogin(force bool) {
	// Check existing auth.
	if !force {
		existing, err := LoadChatGPTAuth()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading auth: %v\n", err)
		}
		if existing != nil && !existing.IsExpired() {
			fmt.Println("Already logged in (token still valid).")
			fmt.Printf("  Account: %s\n", existing.AccountID)
			fmt.Printf("  Expires: %s\n", existing.ExpiresAt.Format(time.RFC3339))
			fmt.Printf("  File:    %s\n", AuthFilePath())
			fmt.Println("\nUse --force to re-login.")
			return
		}
		if existing != nil {
			fmt.Println("Token expired, initiating re-login...")
		}
	}

	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Println("║         OpenAI Codex / ChatGPT Login                ║")
	fmt.Println("╚══════════════════════════════════════════════════════╝")
	fmt.Println()

	// Step 1: Initiate device auth.
	fmt.Print("Requesting device code... ")
	deviceAuth, err := InitiateDeviceAuth()
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n\nFailed to initiate device auth: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("OK")
	fmt.Println()

	// Step 2: Display login instructions.
	authURL := deviceAuth.VerificationURI
	if authURL == "" {
		authURL = codexDeviceURL
	}
	// If there's a verification_uri_complete, prefer it.
	if deviceAuth.VerificationURI != "" && strings.HasPrefix(deviceAuth.VerificationURI, "http") {
		authURL = deviceAuth.VerificationURI
	}

	fmt.Println("┌──────────────────────────────────────────────────────┐")
	fmt.Println("│  1. Buka URL berikut di browser:                     │")
	fmt.Printf("│     %s\n", padRight(authURL, 49))
	fmt.Println("│                                                      │")
	fmt.Println("│  2. Masukkan kode berikut:                            │")
	fmt.Printf("│     %s\n", padRight(deviceAuth.UserCode, 49))
	fmt.Println("│                                                      │")
	fmt.Println("│  3. Klik 'Authorize' untuk memberikan akses          │")
	fmt.Println("└──────────────────────────────────────────────────────┘")
	fmt.Println()
	fmt.Print("Menunggu autentikasi... ")

	// Step 3: Poll for token.
	interval := 5
	if deviceAuth.Interval != "" {
		if i := parseInt(deviceAuth.Interval); i > 0 {
			interval = i
		}
	}

	token, err := PollForToken(deviceAuth.DeviceAuthID, deviceAuth.UserCode, interval)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n\nLogin failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("DONE")
	fmt.Println()

	// Step 4: Build auth object and save.
	accountID := extractAccountID(token.IDToken)
	auth := &ChatGPTAuth{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		IDToken:      token.IDToken,
		AccountID:    accountID,
		ExpiresAt:    time.Now().Add(time.Duration(token.ExpiresIn) * time.Second),
		AuthMode:     "chatgpt",
	}

	if err := SaveChatGPTAuth(auth); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to save auth: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Login berhasil!")
	fmt.Printf("  Account: %s\n", accountID)
	fmt.Printf("  Expires: %s\n", auth.ExpiresAt.Format(time.RFC3339))
	fmt.Printf("  File:    %s\n", AuthFilePath())
	fmt.Println()
	fmt.Println("Anda sekarang bisa menggunakan provider ChatGPT/Codex di aimux.")
}

// padRight pads a string to the given width.
func padRight(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

// parseInt is a simple string-to-int parser.
func parseInt(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}
