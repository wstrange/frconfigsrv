package gitops

import (
	"gopkg.in/src-d/go-git.v4/plumbing"
	"fmt"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"golang.org/x/crypto/ssh"
	"gopkg.in/src-d/go-git.v4"
	gitssh "gopkg.in/src-d/go-git.v4/plumbing/transport/ssh"
	"time"
	"net/http"
	"encoding/json"
	"strings"
	"os"
	"io/ioutil"
)


var (
	gitRepo  *git.Repository
	worktree *git.Worktree
	gitAuth  *gitssh.PublicKeys
	CurrentBranch string   // bit of a hack. We should make this a class???
)


type GitStatusResponse struct {
	Branches   []string `json:"branches"`
	HeadHash   string   `json:"head"`
	HeadBranch string   `json:"headBranch"`
	IsDirty    bool     `json:"isDirty"`
	ChangeCount int `json:"changeCount"`
	FileList	[]string `json:"fileList,omitempty"`
	RemoteList	[]string `json:"remoteList,omitempty"`
	NeedPush	bool    `json:"needPush"`
}

func GitInit(gitroot, branch string) {
	CurrentBranch = branch

	fmt.Printf("Opening git repo at %s branch is %s\n", gitroot, branch)
	repo, err := git.PlainOpen(gitroot)

	CheckIfError(err)
	gitRepo = repo
	worktree, err = gitRepo.Worktree()
	CheckIfError(err)

	// Get the ssh key for git push. This is a well known location within the container
	sshKey, err := ioutil.ReadFile("/etc/git-secret/ssh")
	if err != nil {
		s := fmt.Sprintf("%s/.ssh/id_rsa", os.Getenv("HOME"))
		fmt.Printf("Warning - Can't read default git ssh key at /etc/git-secret/ssh. Defaulting to %s\n", s)
		sshKey, err = ioutil.ReadFile(s)
		CheckIfError(err)
	}
	signer, err := ssh.ParsePrivateKey([]byte(sshKey))
	gitAuth = &gitssh.PublicKeys{User: "git", Signer: signer}
	CheckIfError(err)
}

// Checkout a branch. Try to create it if it does not exist
// The working directory must be clean before you try to switch to a new branch
func GitBranchHandler(w http.ResponseWriter, r *http.Request) {

	err := r.ParseForm()
	reqBranch := r.Form.Get("branch") // x will be "" if parameter is not set

	if err != nil || reqBranch == "" {
		http.Error(w, "Branch parameter missing", http.StatusBadRequest)
		return
	}

	status, err := worktree.Status()

	CheckIfError(err)

	if !status.IsClean() {
		http.Error(w, "Working directory is not clean. Can't switch branches", http.StatusBadRequest)
		return
	}

	branch := fmt.Sprintf("refs/heads/%s", reqBranch)

	fmt.Printf("Checkout Branch %s", branch)

	b := plumbing.ReferenceName(branch)

	// First try to checkout branch
	err = worktree.Checkout(&git.CheckoutOptions{Create: false, Force: false, Branch: b})

	if err != nil {
		// got an error  - try to create it
		fmt.Printf("Error %s - will try creating branch\n", err.Error())
		err := worktree.Checkout(&git.CheckoutOptions{Create: true, Force: false, Branch: b})
		CheckIfError(err)
	}
	fmt.Fprintf(w, "%s", branch)
}

// handle git commit of changed config files. Add any new files before commit
func GitCommitHandler(w http.ResponseWriter, r *http.Request) {
	status, err := worktree.Status()

	CheckIfError(err)

	if status.IsClean() {
		fmt.Fprintf(w, "Nothing to commit")
		return
	}

	for k, v := range status { // foreach changed file....
		if v.Worktree == 'D' {
			fmt.Printf("Removing %s from staging\n", k)
			worktree.Remove(k)
		} else {
			fmt.Printf("Adding %s to staging\n", k)
			worktree.Add(k)
		}
	}

	// create a commit
	commit, err := worktree.Commit("Auto-commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Autosave User",
			Email: "autosave@forgerock.org",
			When:  time.Now(),
		},
	})

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintln(w, "Commit ", commit.String())
}

// handle reset of git files. Removes all files that are not yet in the index
func GitResetHandler(w http.ResponseWriter, r *http.Request) {
	status, err := worktree.Status()

	CheckIfError(err)

	if status.IsClean() {
		fmt.Fprintf(w, "Nothing to commit")
		return
	}

	for k, v := range status { // foreach changed file....
		fmt.Printf("%s %v\n", k, v)
		if v.Worktree == '?' {
			//fmt.Printf("Removing %s from staging %v\n", k, v)
			worktree.Remove(k)
			//fmt.Printf("remove has %s err %v\n", hash, err)
			if err := os.Remove(k); err != nil {
				fmt.Printf("remove err = %v\n", err)
			}

		} else if v.Worktree == 'M' { // todo: this is not handled correctly. We should reset the file.
			hash, err := worktree.Remove(k)
			fmt.Printf("remove has %s err %v\n", hash, err)
		}
	}
	fmt.Fprintf(w,"ok")
}

// handle git push of config files. This assumes the upstream repo has been set already
func GitPushHandler(w http.ResponseWriter, r *http.Request) {
	err := gitRepo.Push(&git.PushOptions{Auth: gitAuth})
	if err != nil {
		fmt.Fprintf(w,"Push error %s", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	} else {
		fmt.Fprintln(w, "Pushed OK")
	}
}

func GitStatusHandler(w http.ResponseWriter, r *http.Request) {

	var gitResponse GitStatusResponse

	fetchFiles := false
	files := r.URL.Query().Get("files")
	if files == "true" {
		fetchFiles = true;
	}

	remotes, err := gitRepo.Remotes()

	gitResponse.RemoteList = make([]string,0,40)

	for _,remote := range remotes {
		c := remote.Config()
		s := fmt.Sprintf("%s %s", c.URL, c.Name)

		gitResponse.RemoteList = append(gitResponse.RemoteList, s)
		// TODO: This is not reliably working
		//err := remote.Fetch( &git.FetchOptions{RemoteName: c.Name, Auth: gitAuth})
		//fmt.Printf("Err = %v", err)
		//if err != nil && err != git.NoErrAlreadyUpToDate {
		//	gitResponse.NeedPush = true;
		//}
	}

	refs, err := gitRepo.References()
	CheckIfError(err)

	err = refs.ForEach(func(ref *plumbing.Reference) error {
		// The HEAD is omitted in a `git show-ref` so we ignore the symbolic
		// references, the HEAD
		if ref.Type() == plumbing.SymbolicReference {
			return nil
		}

		fmt.Println(ref)
		return nil
	})


	b, err := gitRepo.Branches()

	CheckIfError(err)

	s := make([]string, 0, 40)

	b.ForEach(func(reference *plumbing.Reference) error {
		n := reference.Strings()[0]
		s = append(s, strings.TrimPrefix(n, "refs/heads/"))
		return nil
	})

	gitResponse.Branches = s

	status, _ := worktree.Status()
	gitResponse.IsDirty = !status.IsClean()
	gitResponse.ChangeCount = 0

	if !status.IsClean() {
		gitResponse.ChangeCount = len(status)
		if fetchFiles {
			flist := make([]string,0, len(status))
			for k, v := range status { //
				flist = append(flist, fmt.Sprintf("%s %s", k, string(v.Worktree)))
			}
			gitResponse.FileList = flist
		}
	}
	head, err := gitRepo.Head()

	CheckIfError(err)

	gitResponse.HeadHash = head.Strings()[1]
	gitResponse.HeadBranch = strings.TrimPrefix(head.Strings()[0], "refs/heads/")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(gitResponse)
}


func GitListBranches(w http.ResponseWriter, r *http.Request) {
	b, err := gitRepo.Branches()
	CheckIfError(err)

	s := make([]string, 0, 40)

	b.ForEach(func(reference *plumbing.Reference) error {
		n := reference.Strings()[0]
		s = append(s, strings.TrimPrefix(n, "refs/heads/"))
		return nil
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s)

}


func CheckIfError(err error) {
	if err != nil {
		fmt.Printf("Error is %v message %s", err, err.Error())
		panic(err)
	}
}

