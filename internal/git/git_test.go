package git

import "testing"

func TestParseRevParse(t *testing.T) {
	tests := []struct {
		name string
		out  string
		dir  string
		want Repo
	}{
		{
			name: "main checkout has a relative common dir",
			out:  ".git\n/home/u/Projects/app\ndev\n",
			dir:  "/home/u/Projects/app",
			want: Repo{Branch: "dev", RepoName: "app"},
		},
		{
			name: "linked worktree",
			out:  "/home/u/Projects/app/.git\n/home/u/Projects/app/.claude/worktrees/the-site\nworktree-the-site\n",
			dir:  "/home/u/Projects/app/.claude/worktrees/the-site",
			want: Repo{Branch: "worktree-the-site", RepoName: "app", Worktree: "the-site", IsWorktree: true},
		},
		{
			name: "detached HEAD reports no branch",
			out:  ".git\n/home/u/Projects/app\nHEAD\n",
			dir:  "/home/u/Projects/app",
			want: Repo{RepoName: "app"},
		},
		{
			name: "empty output yields the zero value",
			out:  "",
			dir:  "/tmp",
			want: Repo{},
		},
		{
			name: "truncated output yields the zero value",
			out:  ".git\n",
			dir:  "/tmp",
			want: Repo{},
		},
	}
	for _, tt := range tests {
		got := parseRevParse(tt.out, tt.dir)
		if got != tt.want {
			t.Errorf("%s: parseRevParse() = %+v, want %+v", tt.name, got, tt.want)
		}
	}
}
