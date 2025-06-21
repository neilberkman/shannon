package imports

import (
	"crypto/md5"
	"fmt"
	"strings"
	"time"

	"github.com/neilberkman/shannon/internal/models"
)

// BranchDetector detects conversation branches
type BranchDetector struct {
	messages []models.ClaudeChatMessage
	branches []Branch
}

// Branch represents a detected conversation branch
type Branch struct {
	StartIndex  int
	EndIndex    int
	Name        string
	ParentIndex int // -1 for main branch
}

// NewBranchDetector creates a new branch detector
func NewBranchDetector(messages []models.ClaudeChatMessage) *BranchDetector {
	return &BranchDetector{
		messages: messages,
		branches: []Branch{},
	}
}

// DetectBranches analyzes messages to find conversation branches
func (bd *BranchDetector) DetectBranches() []Branch {
	if len(bd.messages) == 0 {
		return bd.branches
	}

	// Method 1: Detect time anomalies (edited messages)
	bd.detectTimeAnomalies()

	// Method 2: Detect duplicate human messages (regenerated responses)
	bd.detectDuplicatePrompts()

	// Method 3: Detect assistant message sequences (multiple responses)
	bd.detectMultipleResponses()

	// If no branches detected, create single main branch
	if len(bd.branches) == 0 {
		bd.branches = append(bd.branches, Branch{
			StartIndex:  0,
			EndIndex:    len(bd.messages) - 1,
			Name:        "main",
			ParentIndex: -1,
		})
	}

	return bd.branches
}

// detectTimeAnomalies looks for messages that are out of chronological order
func (bd *BranchDetector) detectTimeAnomalies() {
	var lastTime time.Time
	branchStart := -1

	for i, msg := range bd.messages {
		msgTime, _ := ParseTime(msg.CreatedAt)

		// Check if this message is earlier than the previous one
		if i > 0 && msgTime.Before(lastTime) {
			// Time went backwards - likely an edit/branch
			if branchStart == -1 {
				branchStart = i
			}
		} else if branchStart != -1 {
			// End of anomaly
			bd.branches = append(bd.branches, Branch{
				StartIndex:  branchStart,
				EndIndex:    i - 1,
				Name:        fmt.Sprintf("edit-%d", len(bd.branches)+1),
				ParentIndex: branchStart - 1,
			})
			branchStart = -1
		}

		lastTime = msgTime
	}
}

// detectDuplicatePrompts looks for repeated human messages
func (bd *BranchDetector) detectDuplicatePrompts() {
	// Map of human message hash to indices
	humanMessages := make(map[string][]int)

	for i, msg := range bd.messages {
		if msg.Sender == "human" {
			// Create hash of message content
			hash := hashMessage(msg.Text)
			humanMessages[hash] = append(humanMessages[hash], i)
		}
	}

	// Find duplicates
	for _, indices := range humanMessages {
		if len(indices) > 1 {
			// Multiple instances of same human message
			for j := 1; j < len(indices); j++ {
				// Each duplicate starts a potential branch
				startIdx := indices[j]
				endIdx := startIdx

				// Find end of branch (next human message or end)
				for k := startIdx + 1; k < len(bd.messages); k++ {
					if bd.messages[k].Sender == "human" {
						endIdx = k - 1
						break
					}
					endIdx = k
				}

				if endIdx > startIdx {
					bd.branches = append(bd.branches, Branch{
						StartIndex:  startIdx,
						EndIndex:    endIdx,
						Name:        fmt.Sprintf("regen-%d", len(bd.branches)+1),
						ParentIndex: indices[0],
					})
				}
			}
		}
	}
}

// detectMultipleResponses looks for multiple assistant messages in a row
func (bd *BranchDetector) detectMultipleResponses() {
	lastHumanIdx := -1
	assistantCount := 0

	for i, msg := range bd.messages {
		switch msg.Sender {
		case "human":
			if assistantCount > 1 {
				// Multiple assistant responses detected
				// This might indicate regenerated responses
				for j := 1; j < assistantCount; j++ {
					bd.branches = append(bd.branches, Branch{
						StartIndex:  lastHumanIdx + j + 1,
						EndIndex:    lastHumanIdx + j + 1,
						Name:        fmt.Sprintf("alt-response-%d", len(bd.branches)+1),
						ParentIndex: lastHumanIdx,
					})
				}
			}
			lastHumanIdx = i
			assistantCount = 0
		case "assistant":
			assistantCount++
		}
	}
}

// hashMessage creates a hash of message content for comparison
func hashMessage(text string) string {
	// Normalize text (lowercase, trim spaces)
	normalized := strings.ToLower(strings.TrimSpace(text))
	hash := md5.Sum([]byte(normalized))
	return fmt.Sprintf("%x", hash)
}
