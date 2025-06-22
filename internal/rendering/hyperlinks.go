package rendering

import (
	"fmt"
	"net/url"
	"strings"
	
	"mvdan.cc/xurls/v2"
)

// MakeHyperlink creates a terminal hyperlink using OSC 8 sequences
// If hyperlinks aren't supported, returns just the display text
func MakeHyperlink(displayText, targetURL string) string {
	if !IsHyperlinksSupported() {
		return displayText
	}
	
	// Validate URL
	if targetURL == "" {
		return displayText
	}
	
	// OSC 8 format: \x1b]8;;URL\x1b\\DISPLAY_TEXT\x1b]8;;\x1b\\
	return fmt.Sprintf("\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\", targetURL, displayText)
}

// MakeHyperlinkWithID creates a hyperlink with an optional ID parameter
func MakeHyperlinkWithID(displayText, targetURL, id string) string {
	if !IsHyperlinksSupported() {
		return displayText
	}
	
	if targetURL == "" {
		return displayText
	}
	
	params := ""
	if id != "" {
		params = "id=" + id
	}
	
	return fmt.Sprintf("\x1b]8;%s;%s\x1b\\%s\x1b]8;;\x1b\\", params, targetURL, displayText)
}

// AutoLinkText automatically converts URLs in text to hyperlinks using xurls
func AutoLinkText(text string) string {
	if !IsHyperlinksSupported() {
		return text
	}
	
	// Use xurls for robust URL detection
	xurlParser := xurls.Relaxed()
	
	return xurlParser.ReplaceAllStringFunc(text, func(match string) string {
		// Ensure URL has a scheme for proper linking
		targetURL := match
		if !strings.HasPrefix(match, "http://") && !strings.HasPrefix(match, "https://") {
			targetURL = "https://" + match
		}
		
		// For auto-linking, use the original text as display, proper URL as target
		return MakeHyperlink(match, targetURL)
	})
}

// MakeLinkedInProfileLink creates a clickable LinkedIn profile link
func MakeLinkedInProfileLink(profileURL string) string {
	if profileURL == "" {
		return ""
	}
	
	// Extract username or show shortened URL for display
	displayText := "LinkedIn Profile"
	
	// Try to extract username from URL for better display
	if parsed, err := url.Parse(profileURL); err == nil {
		path := strings.TrimPrefix(parsed.Path, "/in/")
		path = strings.TrimPrefix(path, "/")
		if path != "" && !strings.Contains(path, "/") {
			displayText = "@" + path
		}
	}
	
	return MakeHyperlink(displayText, profileURL)
}

// MakeCompanyWebsiteLink creates a clickable company website link
func MakeCompanyWebsiteLink(websiteURL, companyName string) string {
	if websiteURL == "" {
		return companyName
	}
	
	displayText := companyName
	if displayText == "" {
		// Fallback to domain name
		if parsed, err := url.Parse(websiteURL); err == nil {
			displayText = parsed.Host
		} else {
			displayText = websiteURL
		}
	}
	
	return MakeHyperlink(displayText, websiteURL)
}

// MakeEmailLink creates a clickable email link
func MakeEmailLink(email string) string {
	if email == "" {
		return ""
	}
	
	return MakeHyperlink(email, "mailto:"+email)
}

// ExtractURLsFromText extracts all URLs from text using xurls
func ExtractURLsFromText(text string) []string {
	xurlParser := xurls.Relaxed()
	return xurlParser.FindAllString(text, -1)
}

// EnhanceTextWithLinks enhances text by making URLs and email addresses clickable
func EnhanceTextWithLinks(text string) string {
	if !IsHyperlinksSupported() {
		return text
	}
	
	// Auto-link URLs and email addresses using xurls
	// The Relaxed parser handles both URLs and email addresses
	text = AutoLinkText(text)
	
	// Note: We removed the fragile regex patterns for GitHub repos and file paths
	// as they were error-prone. xurls will handle github.com URLs properly anyway.
	// If you need special handling for file paths, consider using a proper path
	// parsing library instead of regex.
	
	return text
}