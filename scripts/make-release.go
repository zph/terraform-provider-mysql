package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/hashicorp/go-version"
)

const (
	defaultBranch = "r3.0.62"
	versionFile   = "VERSION"
)

var (
	green  = color.New(color.FgGreen, color.Bold)
	yellow = color.New(color.FgYellow, color.Bold)
	red    = color.New(color.FgRed, color.Bold)
	cyan   = color.New(color.FgCyan, color.Bold)
	blue   = color.New(color.FgBlue, color.Bold)
)

func main() {
	if err := run(); err != nil {
		red.Printf("\n❌ Error: %v\n", err)
		os.Exit(1)
	}
}

var originalBranch string

func run() error {
	// Get the repository root directory
	repoRoot, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("failed to find repository root: %w", err)
	}

	if err := os.Chdir(repoRoot); err != nil {
		return fmt.Errorf("failed to change to repository root: %w", err)
	}

	// Save original branch
	originalBranch, err = getCurrentBranch()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}

	cyan.Println("🚀 Starting release process...")
	fmt.Println()

	// Step 1: Update default branch
	if err := updateDefaultBranch(defaultBranch); err != nil {
		return fmt.Errorf("failed to update default branch: %w", err)
	}

	// Step 2: Read current version
	currentVersion, err := readVersionFile()
	if err != nil {
		return fmt.Errorf("failed to read VERSION file: %w", err)
	}

	// Step 3: Determine tag and version
	tag, versionStr, err := determineTagAndVersion(currentVersion)
	if err != nil {
		return fmt.Errorf("failed to determine tag: %w", err)
	}

	// Step 4: Update VERSION file if needed
	originalVersion := currentVersion
	if versionStr != originalVersion {
		if err := updateVersionFile(versionStr); err != nil {
			return fmt.Errorf("failed to update VERSION file: %w", err)
		}
		green.Printf("✓ Updated VERSION file from %s to %s\n", originalVersion, versionStr)
	}

	// Step 5: Show summary and confirm
	releaseBranch := fmt.Sprintf("release/%s", tag)
	if err := showSummaryAndConfirm(tag, versionStr, releaseBranch, defaultBranch); err != nil {
		return err
	}

	// Step 6: Create release branch
	if err := createReleaseBranch(releaseBranch); err != nil {
		return fmt.Errorf("failed to create release branch: %w", err)
	}

	// Step 7: Commit VERSION file if changed
	if versionStr != originalVersion {
		if err := commitVersionFile(versionStr); err != nil {
			return fmt.Errorf("failed to commit VERSION file: %w", err)
		}
	}

	// Step 8: Create tag
	if err := createTag(tag); err != nil {
		return fmt.Errorf("failed to create tag: %w", err)
	}

	// Step 9: Push branch and tag
	if err := pushBranchAndTag(releaseBranch, tag); err != nil {
		return fmt.Errorf("failed to push: %w", err)
	}

	// Step 10: Show success message
	showSuccessMessage(releaseBranch, defaultBranch, tag)

	return nil
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("not in a git repository")
		}
		dir = parent
	}
}

func updateDefaultBranch(branch string) error {
	cyan.Printf("📥 Updating default branch (%s) before creating release branch...\n", branch)

	// Fetch latest changes
	blue.Println("  Fetching latest changes from origin...")
	if err := runGitCommand("fetch", "origin"); err != nil {
		return fmt.Errorf("git fetch failed: %w", err)
	}

	// Check if branch exists on origin
	if err := runGitCommandSilent("rev-parse", "--verify", fmt.Sprintf("origin/%s", branch)); err != nil {
		return fmt.Errorf("default branch %s does not exist on origin", branch)
	}

	// Checkout or create local branch
	blue.Printf("  Checking out default branch %s...\n", branch)
	if err := runGitCommandSilent("rev-parse", "--verify", branch); err != nil {
		// Branch doesn't exist locally, create it
		if err := runGitCommand("checkout", "-b", branch, fmt.Sprintf("origin/%s", branch)); err != nil {
			return fmt.Errorf("failed to create local branch: %w", err)
		}
	} else {
		// Branch exists, checkout it
		if err := runGitCommand("checkout", branch); err != nil {
			return fmt.Errorf("failed to checkout branch: %w", err)
		}
	}

	// Pull latest changes
	if err := runGitCommand("pull", "origin", branch); err != nil {
		return fmt.Errorf("git pull failed: %w", err)
	}

	green.Printf("✓ Default branch updated successfully.\n\n")
	return nil
}

func readVersionFile() (string, error) {
	data, err := os.ReadFile(versionFile)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func determineTagAndVersion(currentVersion string) (string, string, error) {
	tag := fmt.Sprintf("v%s", currentVersion)
	versionStr := currentVersion

	cyan.Printf("🔍 Checking if tag %s already exists...\n", tag)

	// Check if tag exists
	if err := runGitCommandSilent("rev-parse", tag); err == nil {
		yellow.Printf("⚠ Tag %s already exists!\n", tag)
		cyan.Printf("  Current version from VERSION file: %s\n", currentVersion)

		// Find most recent tag
		lastTag, err := getMostRecentTag()
		if err != nil {
			return "", "", fmt.Errorf("failed to get most recent tag: %w", err)
		}

		if lastTag != "" {
			lastVersion := strings.TrimPrefix(lastTag, "v")
			cyan.Printf("  Most recent tag: %s (version: %s)\n", lastTag, lastVersion)

			// Suggest next version
			nextTag, nextVersion, err := suggestNextVersion(lastVersion)
			if err != nil {
				return "", "", fmt.Errorf("failed to suggest next version: %w", err)
			}

			fmt.Println()
			yellow.Printf("💡 Suggested next tag: %s\n", nextTag)
			fmt.Print("  Enter next tag (or press Enter to use suggested): ")

			reader := bufio.NewReader(os.Stdin)
			userInput, _ := reader.ReadString('\n')
			userInput = strings.TrimSpace(userInput)

			if userInput == "" {
				tag = nextTag
				versionStr = nextVersion
			} else {
				if !strings.HasPrefix(userInput, "v") {
					tag = fmt.Sprintf("v%s", userInput)
				} else {
					tag = userInput
				}
				versionStr = strings.TrimPrefix(tag, "v")
			}
		} else {
			fmt.Println()
			fmt.Print("  Could not determine next tag. Please enter manually: ")
			reader := bufio.NewReader(os.Stdin)
			userInput, _ := reader.ReadString('\n')
			userInput = strings.TrimSpace(userInput)

			if !strings.HasPrefix(userInput, "v") {
				tag = fmt.Sprintf("v%s", userInput)
			} else {
				tag = userInput
			}
			versionStr = strings.TrimPrefix(tag, "v")
		}
	} else {
		green.Printf("✓ Tag %s does not exist. Using version from VERSION file: %s\n", tag, currentVersion)
		versionStr = currentVersion
	}

	fmt.Println()
	return tag, versionStr, nil
}

func getMostRecentTag() (string, error) {
	output, err := runGitCommandOutput("tag", "--list", "v*")
	if err != nil {
		return "", err
	}

	tags := strings.Fields(output)
	if len(tags) == 0 {
		return "", nil
	}

	// Filter tags matching version pattern vX.Y.Z
	versionRegex := regexp.MustCompile(`^v\d+\.\d+\.\d+`)
	var versionTags []string
	for _, tag := range tags {
		if versionRegex.MatchString(tag) {
			versionTags = append(versionTags, tag)
		}
	}

	if len(versionTags) == 0 {
		return "", nil
	}

	// Sort tags by version
	sort.Slice(versionTags, func(i, j int) bool {
		v1, err1 := version.NewVersion(strings.TrimPrefix(versionTags[i], "v"))
		v2, err2 := version.NewVersion(strings.TrimPrefix(versionTags[j], "v"))
		if err1 != nil || err2 != nil {
			return versionTags[i] < versionTags[j]
		}
		return v1.LessThan(v2)
	})

	return versionTags[len(versionTags)-1], nil
}

func suggestNextVersion(lastVersion string) (string, string, error) {
	parts := strings.Split(lastVersion, ".")
	if len(parts) < 3 {
		return "", "", fmt.Errorf("invalid version format: %s", lastVersion)
	}

	buildNum, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return "", "", fmt.Errorf("invalid build number: %w", err)
	}

	majorMinor := strings.Join(parts[:len(parts)-1], ".")
	nextBuild := buildNum + 1
	nextVersion := fmt.Sprintf("%s.%d", majorMinor, nextBuild)
	nextTag := fmt.Sprintf("v%s", nextVersion)

	return nextTag, nextVersion, nil
}

func updateVersionFile(version string) error {
	return os.WriteFile(versionFile, []byte(version+"\n"), 0644)
}

func showSummaryAndConfirm(tag, versionStr, releaseBranch, defaultBranch string) error {
	fmt.Println()
	cyan.Println("════════════════════════════════════════")
	blue.Println("  Release Summary:")
	fmt.Printf("    Tag: %s\n", green.Sprint(tag))
	fmt.Printf("    Version: %s\n", green.Sprint(versionStr))
	fmt.Printf("    Release Branch: %s\n", green.Sprint(releaseBranch))
	fmt.Printf("    Target Branch: %s\n", green.Sprint(defaultBranch))
	cyan.Println("════════════════════════════════════════")
	fmt.Println()

	fmt.Printf("❓ Do you want to create release branch and tag %s? (yes/no): ", tag)
	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	if response != "yes" {
		yellow.Println("Release cancelled.")
		os.Exit(0)
	}

	return nil
}

func createReleaseBranch(branch string) error {
	fmt.Println()
	cyan.Printf("🌿 Creating release branch %s from updated default branch...\n", branch)
	if err := runGitCommand("checkout", "-b", branch); err != nil {
		return fmt.Errorf("failed to create release branch: %w", err)
	}
	green.Printf("✓ Release branch created.\n")
	return nil
}

func commitVersionFile(version string) error {
	fmt.Println()
	cyan.Println("📝 Committing VERSION file change...")
	if err := runGitCommand("add", versionFile); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}
	if err := runGitCommand("commit", "-m", fmt.Sprintf("Update VERSION to %s", version)); err != nil {
		return fmt.Errorf("git commit failed: %w", err)
	}
	green.Printf("✓ VERSION file change committed.\n")
	return nil
}

func createTag(tag string) error {
	fmt.Println()
	cyan.Printf("🏷️  Creating annotated tag %s...\n", tag)
	if err := runGitCommand("tag", "-a", tag, "-m", tag); err != nil {
		return fmt.Errorf("failed to create tag: %w", err)
	}
	green.Printf("✓ Tag %s created successfully.\n", tag)
	return nil
}

func pushBranchAndTag(branch, tag string) error {
	fmt.Println()
	cyan.Println("📤 Pushing branch and tag to origin...")
	if err := runGitCommand("push", "origin", branch); err != nil {
		return fmt.Errorf("failed to push branch: %w", err)
	}
	if err := runGitCommand("push", "origin", tag); err != nil {
		return fmt.Errorf("failed to push tag: %w", err)
	}
	green.Println("✓ Branch and tag pushed successfully.")
	return nil
}

func showSuccessMessage(releaseBranch, defaultBranch, tag string) {
	fmt.Println()
	green.Println("════════════════════════════════════════")
	green.Println("  Release PR Created Successfully!")
	green.Println("════════════════════════════════════════")
	fmt.Println()

	cyan.Println("📋 Next steps:")
	fmt.Println("  1. GitHub Actions will automatically build the release when the tag is pushed.")
	fmt.Println("  2. Create a pull request:")

	// Get repository URL
	repoURL, err := getRepositoryURL()
	if err == nil {
		fmt.Printf("     - Source: %s\n", releaseBranch)
		fmt.Printf("     - Target: %s\n", defaultBranch)
		fmt.Printf("     - URL: https://github.com/%s/compare/%s...%s\n", repoURL, defaultBranch, releaseBranch)
	} else {
		fmt.Printf("     - Source: %s\n", releaseBranch)
		fmt.Printf("     - Target: %s\n", defaultBranch)
	}

	fmt.Println("  3. Wait for CI to complete the release build.")
	fmt.Printf("  4. Review and merge the PR into %s to complete the release.\n", defaultBranch)
	fmt.Println()

	// Suggest switching back to original branch
	if originalBranch != "" && originalBranch != releaseBranch {
		yellow.Printf("💡 To switch back to your previous branch, run:\n")
		fmt.Printf("     git checkout %s\n", originalBranch)
	}
}

func getRepositoryURL() (string, error) {
	output, err := runGitCommandOutput("config", "--get", "remote.origin.url")
	if err != nil {
		return "", err
	}

	url := strings.TrimSpace(output)
	// Handle both SSH and HTTPS URLs
	url = strings.TrimSuffix(url, ".git")
	if strings.Contains(url, "github.com:") {
		// SSH format: git@github.com:user/repo
		parts := strings.Split(url, ":")
		if len(parts) > 1 {
			return parts[len(parts)-1], nil
		}
	} else if strings.Contains(url, "github.com/") {
		// HTTPS format: https://github.com/user/repo
		parts := strings.Split(url, "github.com/")
		if len(parts) > 1 {
			return parts[len(parts)-1], nil
		}
	}

	return "", fmt.Errorf("could not parse repository URL")
}

func getCurrentBranch() (string, error) {
	output, err := runGitCommandOutput("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func runGitCommand(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runGitCommandSilent(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

func runGitCommandOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	output, err := cmd.Output()
	return strings.TrimSpace(string(output)), err
}
