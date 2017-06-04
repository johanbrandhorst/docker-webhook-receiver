package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/fsouza/go-dockerclient"
)

// ContainerName is the name of the container to restart
const ContainerName = "app"

// ContainerRepository is the container repository to use
const ContainerRepository = "jfbrandhorst/grpcweb-example"

// DockerHubWebhook is the structure of the JSON sent by a DockerHub Webhook
type DockerHubWebhook struct {
	PushData struct {
		PushedAt int      `json:"pushed_at"`
		Images   []string `json:"images"`
		Tag      string   `json:"tag"`
		Pusher   string   `json:"pusher"`
	} `json:"push_data"`
	CallbackURL string `json:"callback_url"`
	Repository  struct {
		Status          string `json:"status"`
		Description     string `json:"description"`
		IsTrusted       bool   `json:"is_trusted"`
		FullDescription string `json:"full_description"`
		RepoURL         string `json:"repo_url"`
		Owner           string `json:"owner"`
		IsOfficial      bool   `json:"is_official"`
		IsPrivate       bool   `json:"is_private"`
		Name            string `json:"name"`
		Namespace       string `json:"namespace"`
		StarCount       int    `json:"star_count"`
		CommentCount    int    `json:"comment_count"`
		DateCreated     int    `json:"date_created"`
		RepoName        string `json:"repo_name"`
	} `json:"repository"`
}

// HookState describes the state of the hook
type HookState string

// Allowed states of HookState
const (
	Success = HookState("success")
	Failure = HookState("failure")
	Error   = HookState("error")
)

// DockerCallback is the structure of the callback reply
type DockerCallback struct {
	State       HookState `json:"state"`
	Description string    `json:"description"`
	Context     string    `json:"context"`
	TargetURL   string    `json:"target_url"`
}

// WebhookHandler handles requests
type WebhookHandler struct {
	client *docker.Client
}

func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	content, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Print(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	hook := DockerHubWebhook{}
	err = json.Unmarshal(content, &hook)
	if err != nil {
		log.Print(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if !strings.HasPrefix(hook.CallbackURL, "https://registry.hub.docker.com/u/jfbrandhorst/grpcweb-example") {
		log.Print("Got request not from docker hub")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	reply := DockerCallback{
		State:       Success,
		Description: "Redeploy was successful",
		Context:     "docker-webhook-receiver",
		TargetURL:   "https://demo.jbrandhorst.com",
	}
	respBytes, err := json.Marshal(&reply)
	if err != nil {
		log.Print(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	_, err = http.Post(hook.CallbackURL, "application/json", bytes.NewReader(respBytes))
	if err != nil {
		log.Print(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// At this point we can be sure this was a genuine request, because
	// the CallbackURL worked.
	err = h.client.StopContainer(ContainerName, 5)
	if err != nil {
		log.Print(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = h.client.RemoveContainer(docker.RemoveContainerOptions{
		ID:            ContainerName,
		RemoveVolumes: true,
	})
	if err != nil {
		log.Print(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = h.client.PullImage(docker.PullImageOptions{
		Repository: ContainerRepository,
		Tag:        "latest",
	}, docker.AuthConfiguration{})
	if err != nil {
		log.Print(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	container, err := h.client.CreateContainer(docker.CreateContainerOptions{
		Name: ContainerName,
		Config: &docker.Config{
			Image:        ContainerRepository,
			AttachStderr: true,
			AttachStdout: true,
			Cmd: []string{
				"--host",
				"demo.jbrandhorst.com",
			},
		},
		HostConfig: &docker.HostConfig{
			PortBindings: map[docker.Port][]docker.PortBinding{
				docker.Port("443"): []docker.PortBinding{
					{HostPort: "443"},
				},
			},
		},
	})
	if err != nil {
		log.Print(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = h.client.StartContainer(container.ID, nil)
	if err != nil {
		log.Print(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	log.Print("Container restarted successfully")
	w.WriteHeader(http.StatusOK)
}

var log *logrus.Logger

func init() {
	log = logrus.StandardLogger()
	logrus.SetLevel(logrus.InfoLevel)
	logrus.SetFormatter(&logrus.TextFormatter{
		ForceColors:     true,
		FullTimestamp:   true,
		TimestampFormat: time.RFC1123,
		DisableSorting:  true,
	})
}

func main() {
	flag.Parse()

	client, err := docker.NewClientFromEnv()
	if err != nil {
		log.Fatal("Failed to create docker client:", err)
	}

	handler := &WebhookHandler{
		client: client,
	}

	http.HandleFunc("/docker-webhook", handler.ServeHTTP)
	log.Print("Serving on http://0.0.0.0:8080")
	log.Fatal(http.ListenAndServe("0.0.0.0:8080", nil))
}
