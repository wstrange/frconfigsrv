package main

// Utility that listens for requests to a) export configuration (amster only)
// and b) git commit / push that configuration
// This assumes the git project has already been checked out (probably by an init container or amster)
// And that we are on the right branch

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/cors"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"forgerock.io/frconfigsrv/gitops"
)

// Holds our defaults
type Config struct {
	ExportPath   string `json:"exportPath"`
	GitPath      string `json:"gitRootDirectory"`
	GitBranch    string `json:"gitBranch"`
	IsAutoExport bool   `json:"isAutoExport"`
}

var (
	config   Config
)

func paramValue(param string, defValue string) string {
	if len(param) > 0 {
		return param
	}
	return defValue
}

type flushWriter struct {
	f http.Flusher
	w io.Writer
}

func (fw *flushWriter) Write(p []byte) (n int, err error) {
	n, err = fw.w.Write(p)
	if fw.f != nil {
		fw.f.Flush()
	}
	return
}

// Export configuration. This only applies to Amster - as IDM is assumed to be configured to
// export all the time. TODO: We might want to see if can update IDM on the fly
func exportHandler(w http.ResponseWriter, r *http.Request) {

	if config.IsAutoExport {
		fmt.Fprintln(w, "Configuration is auto exported. Manual export not supported")
		return
	}

	path := r.URL.Query().Get("path")

	if path == "" {
		path = config.ExportPath;
	}

	// do the export - we just handle amster right now
	exportPath := fmt.Sprintf("%s/%s", strings.TrimSuffix(config.GitPath, "/"), path)
	//fmt.Printf("Path %s\n", exportPath)
	amsterScript := "/tmp/export.amster"

	// create the amster export script that exports to the path
	s := fmt.Sprintf("connect -k /var/run/secrets/amster/id_rsa http://openam/openam\nexport-config --path %s\n:quit\n",
		exportPath)
	ioutil.WriteFile(amsterScript, []byte(s), 0644)

	fmt.Printf("Executing amster export to %s\n", exportPath)
	cmd := exec.Command("/opt/amster/amster", amsterScript)

	execRequest(cmd, w, r)
}

// Shell out to the command and stream the results back to the browser
func execRequest(cmd *exec.Cmd, w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Exec command %v\n", cmd.Args)
	fw := flushWriter{w: w}
	if f, ok := w.(http.Flusher); ok {
		fw.f = f
	}
	cmd.Stdout = &fw
	cmd.Stderr = &fw
	fmt.Fprintln(cmd.Stdout, "Output:") // triggers a flush of the browser
	// exec the command and stream the results back to the browser
	if err := cmd.Run(); err != nil {
		s := fmt.Sprintf("OS Error: %v\n", err)
		fmt.Println(s)
		// triggers go error http: multiple response.WriteHeader calls
		// This is because the headers have already been sent at the start of the stream. We can't
		// change them after.
		// If we need to returns a non 200 error, we could use http trailers
		//http.Error(w,s,500)
		fmt.Fprintf(cmd.Stdout, s)
	}
}


// A very simple index in case there is no GUI installed
func index(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, `<html>
<body>
<ul>
<li><a href="/ui/">UI</a></li>
<li><a href="/export">Amster - export current configuration</a></li>
<li><a href="/git/status">Git status</a></li>
<li><a href="/git/branch">Git change to configured branch</a></li>
<li><a href="/git/commit">Git add and commit configuration changes locally</a></li>
<li><a href="/git/push">Git push commited changes upstream</a></li>
</ul>
</body>
</html>
`)
}

func getConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

// TODO: What do we want to allow the user to change in the config?
// Right now, just the git branch and the export path (amster only)
func setConfig(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	var newConfig Config
	err := decoder.Decode(&newConfig)
	CheckIfError(err)
	// now see what we can change...
	if len(newConfig.ExportPath) > 5 {
		config.ExportPath = newConfig.ExportPath
		// todo: Do we want to see if we can write to the path?, or let amster handle that?
	}
	if len(newConfig.GitBranch) > 1 {
		// switch branches...
		config.GitBranch = newConfig.GitBranch
	}
	fmt.Fprintf(w, "Config is now %v", config)
}

func main() {
	var address = ":9080" //default listen address

	if len(os.Args) >= 2 {
		address = os.Args[1]
	}

	fmt.Printf("Listen address %s\n", address)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// CORS is needed as we may host the GUI outside of the cluster
	cors := cors.New(cors.Options{ // AllowedOrigins: []string{"https://foo.com"}, // Use this to allow specific origin hosts
		AllowedOrigins: []string{"*"},
		// AllowOriginFunc:  func(r *http.Request, origin string) bool { return true },
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		//ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300, // Maximum value not ignored by any of major browsers
	})

	r.Use(cors.Handler)
	r.Get("/", index)

	// Read our configuration from ENV variables.
	config.ExportPath = paramValue(os.Getenv("EXPORT_PATH"), "default/am/autosave-am")
	gitRoot := paramValue(os.Getenv("GIT_ROOT"), "/git")
	projectDirectory :=  paramValue(os.Getenv("GIT_PROJECT_DIRECTORY"), "forgeops-init")
	config.GitPath = fmt.Sprintf("%s/%s", gitRoot, projectDirectory)
	config.GitBranch = paramValue(os.Getenv("GIT_AUTOSAVE_BRANCH"), "autosave")

	// If we find "amster" the configuration is not auto -exported. This flag tells us if we should run amster export
	config.IsAutoExport = false
	if _, err := os.Stat("/opt/amster/amster"); os.IsNotExist(err) {
		config.IsAutoExport = true // we must be running in the IDM container - not amster
	}

	r.Get("/export", exportHandler)
	r.Route("/git", func(r chi.Router) {
		r.Get("/status", gitops.GitStatusHandler)
		r.Get("/commit", gitops.GitCommitHandler)
		r.Get("/push", gitops.GitPushHandler)
		r.Get("/branch", gitops.GitListBranches)
		r.Post("/branch", gitops.GitBranchHandler)
		r.Post("/reset", gitops.GitResetHandler)
	})
	r.Route("/config", func(r chi.Router) {
		r.Get("/", getConfig)
		r.Post("/", setConfig)
	})

	gitops.GitInit(config.GitPath, config.GitBranch)

	// setup ui server
	workDir, _ := os.Getwd()
	filesDir := filepath.Join(workDir, "ui/build/web")
	FileServer(r, "/ui", http.Dir(filesDir))

	// change to git path
	if err := os.Chdir(config.GitPath); err != nil {
		panic( fmt.Sprintf("Can't change working directory to %s. err=%s\n", config.GitPath, err  ) )
	}

	err := http.ListenAndServe(address, r)
	fmt.Printf("Listening on  %s. err = %v\n", address, err)
}

// Serve static files from a http.FileSystem. Used for GUI if it is installed
func FileServer(r chi.Router, path string, root http.FileSystem) {
	if strings.ContainsAny(path, "{}*") {
		panic("FileServer does not permit URL parameters.")
	}

	fs := http.StripPrefix(path, http.FileServer(root))

	if path != "/" && path[len(path)-1] != '/' {
		r.Get(path, http.RedirectHandler(path+"/", 301).ServeHTTP)
		path += "/"
	}
	path += "*"

	r.Get(path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs.ServeHTTP(w, r)
	}))
}

func CheckIfError(err error) {
	if err != nil {
		fmt.Printf("Error is %v message %s", err, err.Error())
		panic(err)
	}
}
