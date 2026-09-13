package crawler

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

type RepoChange struct {
	Repo    RepoInfo
	OldSHA  string
	NewSHA  string
	Changed bool
}

func GetHeadSHA(repoPath string) string {
	out, err := exec.Command("git", "-C", repoPath, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func PullRepo(repoPath string) error {
	cmd := exec.Command("git", "-C", repoPath, "pull", "--ff-only", "--quiet")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git pull %s: %s", repoPath, string(out))
	}
	return nil
}

func PullAll(repos []RepoInfo, storedSHAs map[string]string, concurrency int) []RepoChange {
	if concurrency <= 0 {
		concurrency = 20
	}

	changes := make([]RepoChange, len(repos))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i, repo := range repos {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, r RepoInfo) {
			defer wg.Done()
			defer func() { <-sem }()

			oldSHA := storedSHAs[ServiceKey(r.Workspace, r.Name)]
			_ = PullRepo(r.Path)
			newSHA := GetHeadSHA(r.Path)

			changes[idx] = RepoChange{
				Repo:    r,
				OldSHA:  oldSHA,
				NewSHA:  newSHA,
				Changed: oldSHA != newSHA,
			}
		}(i, repo)
	}
	wg.Wait()
	return changes
}
