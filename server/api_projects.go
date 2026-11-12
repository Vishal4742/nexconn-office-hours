package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"officehours/github"
	"officehours/guard"
	"officehours/store"
)

// botID is the Nexconn user that owns every room and sends every OH:* message. No person can be
// it: people are "gh" followed by a number.
const botID = "oh_bot"

const (
	maxDescription = 350 // GitHub's own limit for a repository's description
	// ponytail: Nexconn documents no limit for a group's name. 64 is safe and loses nothing we
	// show (the pages take the name from our own tables). Raise it if a limit is ever documented.
	maxGroupName = 64

	projectsNotSetUp = "Projects aren't set up on this server yet."
	githubSilent     = "GitHub didn't answer. Try again in a moment."
)

// roomService is the part of the Nexconn client that sets up a project's room.
type roomService interface {
	CreateGroup(ctx context.Context, channelID, name, ownerID string) error
	JoinGroup(ctx context.Context, channelID string, userIDs ...string) error
}

// projectStore is the part of the store the project routes need.
type projectStore interface {
	ProjectByRepo(ctx context.Context, owner, repo, viewerID string) (store.Project, bool, error)
	CreateProject(ctx context.Context, p store.Project, creatorID string) (store.Project, bool, error)
	AddMaintainer(ctx context.Context, projectID, userID string) error
	ProjectsOf(ctx context.Context, userID string) ([]store.Project, error)
}

// errRoom marks a failure of the chat service while a room was being set up.
var errRoom = errors.New("chat room setup failed")

// errOtherRepo means the name belongs to a project that was registered for another repository.
var errOtherRepo = errors.New("the name is registered for another repository")

// groupID is the Nexconn group channel of a project's room. Project ids are random, so nobody can
// name a room's group before the server has created it (see nexconn.CreateGroup).
func groupID(projectID string) string { return "p" + projectID }

// projectJSON is a project as the API describes it to one caller.
type projectJSON struct {
	ID           string `json:"id,omitempty"` // left out for signed-out visitors: it names the chat room
	Owner        string `json:"owner"`
	Repo         string `json:"repo"`
	Description  string `json:"description"`
	IsMember     bool   `json:"isMember"`
	IsMaintainer bool   `json:"isMaintainer"`
}

func describe(p store.Project) projectJSON {
	return projectJSON{p.ID, p.Owner, p.Repo, p.Description, p.IsMember, p.IsMaintainer}
}

// registerProject makes a repository the caller maintains a project with a room (FR-PROJ-1,
// FR-PROJ-2). Registering one that exists returns it, and makes the caller a maintainer of it if
// GitHub says they are one (plan decision 4).
func (d deps) registerProject(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Owner string `json:"owner"`
		Repo  string `json:"repo"`
	}
	u, err := d.guard.Mutation(w, r, &in)
	if err != nil {
		writeError(w, err)
		return
	}
	if !validOwnerRepo(in.Owner, in.Repo) {
		writeJSON(w, http.StatusBadRequest, envelope{Error: "That is not a GitHub repository. Write it as owner/repo, like paperplane/stash."})
		return
	}
	if d.github == nil || d.projects == nil || d.rooms == nil {
		writeJSON(w, http.StatusServiceUnavailable, envelope{Error: "Registering a repo isn't set up on this server yet."})
		return
	}

	asked := in.Owner + "/" + in.Repo
	repo, err := d.github.Repo(r.Context(), u.GitHubToken, in.Owner, in.Repo)
	if refusal := repoRefusal(asked, repo, err); refusal != nil {
		writeError(w, refusal)
		return
	}
	project, created, err := d.projectFor(r.Context(), repo, u.ID)
	switch {
	case errors.Is(err, errRoom):
		log.Printf("register %s: %v", asked, err)
		writeJSON(w, http.StatusBadGateway, envelope{Error: "We couldn't set up the chat room. Try again in a moment."})
		return
	case errors.Is(err, errOtherRepo):
		log.Printf("register %s: refused, the name is registered for another GitHub repository", asked)
		writeJSON(w, http.StatusConflict, envelope{Error: repo.Owner + "/" + repo.Name + " was registered here for a different GitHub repository that had this name before. This one can't take over its room."})
		return
	case err != nil:
		writeError(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, envelope{OK: true, Data: describe(project)})
}

// repoRefusal says why the caller may not register the repository GitHub described, or nil when
// they may (FR-PROJ-1). asked is the name as it was typed, for the replies that carry none.
func repoRefusal(asked string, repo github.Repo, err error) *guard.Error {
	name := repo.Owner + "/" + repo.Name
	switch {
	case errors.Is(err, github.ErrNotFound):
		return &guard.Error{Status: http.StatusNotFound, Msg: "We couldn't find a public repository called " + asked + " on GitHub. Check the spelling. Private repositories aren't supported."}
	case errors.Is(err, github.ErrUnauthorized):
		// GitHub keeps ten tokens per user and app and revokes the oldest, so this happens to
		// people who sign in on many devices.
		return &guard.Error{Status: http.StatusUnauthorized, Msg: "GitHub no longer accepts your sign-in. Sign in again, then register the repo."}
	case errors.Is(err, github.ErrForbidden):
		return &guard.Error{Status: http.StatusForbidden, Msg: "GitHub refused to show us " + asked + ". If it belongs to an organization, ask one of its owners to approve Office Hours, then try again."}
	case err != nil:
		log.Printf("register %s: %v", asked, err)
		return &guard.Error{Status: http.StatusBadGateway, Msg: githubSilent}
	case !validOwnerRepo(repo.Owner, repo.Name):
		// The names go into our URLs and tables: what GitHub sends is checked like any input.
		log.Printf("register %s: GitHub's reply names a repository we can't use", asked)
		return &guard.Error{Status: http.StatusBadGateway, Msg: githubSilent}
	case repo.Private:
		return &guard.Error{Status: http.StatusForbidden, Msg: name + " is private. Office Hours only works with public repositories."} // NFR-PRIV-1
	case !repo.KnowsPermissions:
		log.Printf("register %s: assumption A-7 failed: GitHub's reply has no permissions", asked)
		return &guard.Error{Status: http.StatusBadGateway, Msg: "GitHub didn't say what rights you have on " + name + ", so we can't tell that you maintain it. Try again in a moment."}
	case !repo.CanMaintain && repo.OwnerIsOrg:
		// An organization that restricts OAuth apps makes its own maintainers look like outsiders.
		return &guard.Error{Status: http.StatusForbidden, Msg: "GitHub says you don't have admin or maintain rights on " + name + ". If you do, ask an owner of " + repo.Owner + " to approve Office Hours for the organization, then try again."}
	case !repo.CanMaintain:
		return &guard.Error{Status: http.StatusForbidden, Msg: "GitHub says you don't have admin or maintain rights on " + name + ". Only its maintainers can host office hours for it."}
	}
	return nil
}

// projectFor returns the project of a repository GitHub says the user maintains, creating it if
// this is its first registration, with the user as a maintainer and a member of its room.
//
// The room always comes first, the rows second (plan decision 3): a project row means a group we
// created, and a maintainer row means that person is in it. A failure in between leaves a group
// nobody uses, which costs nothing and needs no repair.
func (d deps) projectFor(ctx context.Context, repo github.Repo, userID string) (project store.Project, created bool, err error) {
	project, found, err := d.projects.ProjectByRepo(ctx, repo.Owner, repo.Name, userID)
	if err != nil {
		return store.Project{}, false, err
	}
	if !found {
		fresh := store.Project{
			ID:       strings.ToLower(rand.Text()), // 26 random characters: nobody can guess the group's id
			GitHubID: repo.ID,
			Owner:    repo.Owner, Repo: repo.Name, Description: clip(repo.Description, maxDescription),
		}
		if err := d.rooms.CreateGroup(ctx, groupID(fresh.ID), clip(fresh.Owner+"/"+fresh.Repo, maxGroupName), botID); err != nil {
			return store.Project{}, false, fmt.Errorf("%w: %v", errRoom, err)
		}
		if err := d.rooms.JoinGroup(ctx, groupID(fresh.ID), userID); err != nil {
			return store.Project{}, false, fmt.Errorf("%w: %v", errRoom, err)
		}
		if project, created, err = d.projects.CreateProject(ctx, fresh, userID); err != nil {
			return store.Project{}, false, err
		}
		// Not created: someone registered the same repo a moment ago. Their project stands, and
		// the caller joins it below like any second maintainer.
	}
	// A name can change hands: the first repo is renamed or deleted and someone makes a new one
	// under its name. Being admin of that one must not make them a maintainer of this project.
	if project.GitHubID != repo.ID {
		return store.Project{}, false, errOtherRepo
	}
	if !project.IsMember || !project.IsMaintainer {
		if err := d.rooms.JoinGroup(ctx, groupID(project.ID), userID); err != nil {
			return store.Project{}, false, fmt.Errorf("%w: %v", errRoom, err)
		}
		if err := d.projects.AddMaintainer(ctx, project.ID, userID); err != nil {
			return store.Project{}, false, err
		}
		project.IsMember, project.IsMaintainer = true, true
	}
	return project, created, nil
}

// projectPage is what the page /p/{owner}/{repo} needs first (FR-PROJ-3). It is public. A
// signed-in caller also learns the project's id and what they are in it.
func (d deps) projectPage(w http.ResponseWriter, r *http.Request) {
	owner, repo := r.PathValue("owner"), r.PathValue("repo")
	if !validOwnerRepo(owner, repo) {
		writeJSON(w, http.StatusNotFound, envelope{Error: "There is no such project."})
		return
	}
	if d.projects == nil {
		writeJSON(w, http.StatusServiceUnavailable, envelope{Error: projectsNotSetUp})
		return
	}
	viewer, _ := d.guard.User(r) // signed out is fine here: the zero user is nobody
	project, found, err := d.projects.ProjectByRepo(r.Context(), owner, repo, viewer.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, envelope{Error: "Nobody hosts office hours for " + owner + "/" + repo + " yet."})
		return
	}
	out := describe(project)
	if viewer.ID == "" {
		out.ID = ""
	}
	writeJSON(w, http.StatusOK, envelope{OK: true, Data: out})
}

// myProjects lists the rooms the caller is in, for the dashboard.
func (d deps) myProjects(w http.ResponseWriter, r *http.Request) {
	u, err := d.guard.User(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if d.projects == nil {
		writeJSON(w, http.StatusServiceUnavailable, envelope{Error: projectsNotSetUp})
		return
	}
	projects, err := d.projects.ProjectsOf(r.Context(), u.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]projectJSON, 0, len(projects)) // an empty list is [] in JSON, never null
	for _, p := range projects {
		out = append(out, describe(p))
	}
	writeJSON(w, http.StatusOK, envelope{OK: true, Data: out})
}
