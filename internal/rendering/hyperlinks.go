package rendering

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
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

// AutoLinkText automatically converts URLs in text to hyperlinks
func AutoLinkText(text string) string {
	if !IsHyperlinksSupported() {
		return text
	}
	
	// Regex to match URLs (basic version)
	urlRegex := regexp.MustCompile(`https?://[^\s<>"{}|\\^` + "`" + `\[\]]+`)
	
	return urlRegex.ReplaceAllStringFunc(text, func(match string) string {
		// For auto-linking, use the URL as both display text and target
		return MakeHyperlink(match, match)
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

// ExtractURLsFromText extracts all URLs from text for processing
func ExtractURLsFromText(text string) []string {
	urlRegex := regexp.MustCompile(`https?://[^\s<>"{}|\\^` + "`" + `\[\]]+`)
	return urlRegex.FindAllString(text, -1)
}

// EnhanceTextWithLinks enhances text by making various patterns clickable
func EnhanceTextWithLinks(text string) string {
	if !IsHyperlinksSupported() {
		return text
	}
	
	// Auto-link URLs
	text = AutoLinkText(text)
	
	// Auto-link email addresses
	emailRegex := regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`)
	text = emailRegex.ReplaceAllStringFunc(text, func(email string) string {
		return MakeEmailLink(email)
	})
	
	// Auto-link GitHub repositories (github.com/user/repo)
	githubRegex := regexp.MustCompile(`github\.com/([A-Za-z0-9._-]+)/([A-Za-z0-9._-]+)`)
	text = githubRegex.ReplaceAllStringFunc(text, func(match string) string {
		// Extract user and repo from the match
		parts := strings.Split(match, "/")
		if len(parts) >= 3 {
			user := parts[1]
			repo := parts[2]
			displayText := fmt.Sprintf("%s/%s", user, repo)
			return MakeHyperlink(displayText, "https://"+match)
		}
		return match
	})
	
	// Auto-link file paths (starting with ./ or /)
	fileRegex := regexp.MustCompile(`(?:^|\s)((?:\./|/)[^\s<>"{}|\\^` + "`" + `\[\]]+)`)
	text = fileRegex.ReplaceAllStringFunc(text, func(match string) string {
		trimmed := strings.TrimSpace(match)
		if strings.HasPrefix(trimmed, "./") || strings.HasPrefix(trimmed, "/") {
			return strings.Replace(match, trimmed, MakeHyperlink(trimmed, "file://"+trimmed), 1)
		}
		return match
	})
	
	return text
}