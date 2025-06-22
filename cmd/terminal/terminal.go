package terminal

import (
	"fmt"

	"github.com/neilberkman/shannon/internal/rendering"
	"github.com/spf13/cobra"
)

// TerminalCmd represents the terminal command
var TerminalCmd = &cobra.Command{
	Use:   "terminal",
	Short: "Show terminal capabilities and features",
	Long: `Display information about the current terminal's capabilities and which Shannon features are available.

This command helps you understand what advanced features like hyperlinks and graphics are supported in your terminal.`,
	RunE: runTerminal,
}

func runTerminal(cmd *cobra.Command, args []string) error {
	caps := rendering.DetectTerminalCapabilities()

	fmt.Println("Terminal Information:")
	fmt.Printf("  Type: %s\n", caps.TerminalType)
	fmt.Println()

	fmt.Println("Supported Features:")

	if caps.SupportsHyperlinks {
		fmt.Println("  ✓ OSC 8 Hyperlinks - Clickable links in search results and conversation lists")

		// Show a demo hyperlink
		if rendering.IsHyperlinksSupported() {
			demoLink := rendering.MakeHyperlink("Click here for Shannon documentation", "https://github.com/neilberkman/shannon")
			fmt.Printf("    Demo: %s\n", demoLink)
		}
	} else {
		fmt.Println("  ✗ OSC 8 Hyperlinks - Not supported")
	}

	if caps.SupportsGraphics {
		fmt.Println("  ✓ Graphics Protocol - Image display support (Kitty Graphics Protocol)")
		fmt.Println("    Note: Graphics features not yet implemented in Shannon")
	} else {
		fmt.Println("  ✗ Graphics Protocol - Not supported")
	}

	if caps.SupportsAdvancedInput {
		fmt.Println("  ✓ Advanced Input - Enhanced keyboard handling")
	} else {
		fmt.Println("  ✗ Advanced Input - Basic keyboard only")
	}

	fmt.Println()
	fmt.Println("Recommendations:")

	if caps.TerminalType == "ghostty" {
		fmt.Println("  🎉 You're using Ghostty! All Shannon features are optimally supported.")
	} else if caps.SupportsHyperlinks {
		fmt.Println("  👍 Your terminal supports hyperlinks. Enjoy clickable search results!")
	} else {
		fmt.Println("  💡 For the best Shannon experience, try:")
		fmt.Println("     - Ghostty (https://ghostty.org)")
		fmt.Println("     - Kitty (https://sw.kovidgoyal.net/kitty/)")
		fmt.Println("     - WezTerm (https://wezfurlong.org/wezterm/)")
	}

	return nil
}
