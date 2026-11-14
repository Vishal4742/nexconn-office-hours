package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrUnknownUser means a row named a user who has no row of their own: a session that outlived
// its account. Signing in again fixes that, so callers tell it apart from a broken database.
var ErrUnknownUser = errors.New("store: no such user")

// Project is one registered repository. IsMember and IsMaintainer describe the user a query was
// asked for, and are false when it was asked for nobody.
type Project struct {
	ID           string // random; the room's Nexconn group is "p" + ID
	GitHubID     int64  // GitHub's id of the repository it was registered for
	Owner        string // GitHub's own spelling
	Repo         string
	Description  string
	IsMember     bool
	IsMaintainer bool
}

// rowQuerier is what the pool and a transaction have in common.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// projectSelect reads a project as the user $1 sees it. An empty $1 is a signed-out visitor.
const projectSelect = `
	SELECT p.id, p.github_id, p.owner, p.repo, p.description,
	       EXISTS (SELECT 1 FROM members m WHERE m.project_id = p.id AND m.user_id = $1),
	       EXISTS (SELECT 1 FROM maintainers t WHERE t.project_id = p.id AND t.user_id = $1)
	FROM projects p `

// scanProject reads the one row of a projectSelect query; no row is "not found", not an error.
func scanProject(row pgx.Row) (Project, bool, error) {
	var p Project
	err := row.Scan(&p.ID, &p.GitHubID, &p.Owner, &p.Repo, &p.Description, &p.IsMember, &p.IsMaintainer)
	if errors.Is(err, pgx.ErrNoRows) {
		return Project{}, false, nil
	}
	if err != nil {
		return Project{}, false, fmt.Errorf("store: find project: %w", err)
	}
	return p, true, nil
}

// ProjectByRepo finds a project by the name it is registered under, in any letter case, and says
// what viewerID is in it. This is how pages find a project: by the address they were asked for.
func (s *Store) ProjectByRepo(ctx context.Context, owner, repo, viewerID string) (Project, bool, error) {
	return scanProject(s.pool.QueryRow(ctx, projectSelect+
		`WHERE lower(p.owner) = lower($2::text) AND lower(p.repo) = lower($3::text)`,
		viewerID, owner, repo))
}

// ProjectOf finds the project of a GitHub repository (FR-PROJ-2): the one registered for this
// repository id, whatever name it has here, or else the one that holds this name. In the second
// case its GitHubID differs from the one asked for: the name belongs to another repository.
func (s *Store) ProjectOf(ctx context.Context, githubID int64, owner, repo, viewerID string) (Project, bool, error) {
	return projectOf(ctx, s.pool, githubID, owner, repo, viewerID)
}

func projectOf(ctx context.Context, q rowQuerier, githubID int64, owner, repo, viewerID string) (Project, bool, error) {
	return scanProject(q.QueryRow(ctx, projectSelect+
		`WHERE p.github_id = $2 OR (lower(p.owner) = lower($3::text) AND lower(p.repo) = lower($4::text))
		 ORDER BY (p.github_id = $2) DESC
		 LIMIT 1`,
		viewerID, githubID, owner, repo))
}

// addMaintainerSQL makes a user a maintainer and a member in one statement, so both rows exist or
// neither. Someone who is one or both already stays as they are.
const addMaintainerSQL = `
	WITH added AS (
		INSERT INTO maintainers (project_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING
	)
	INSERT INTO members (project_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`

// CreateProject records a new project with creatorID as its first maintainer and member
// (FR-PROJ-2). If the repository is registered already, or its name is taken, it writes nothing
// and returns the project that is in the way (see ProjectOf) with created false. Of two
// registrations at the same moment one wins and the other learns the winner's project: the
// unique indexes on the repository's id and on its name decide.
func (s *Store) CreateProject(ctx context.Context, p Project, creatorID string) (project Project, created bool, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Project{}, false, fmt.Errorf("store: create project: %w", err)
	}
	defer tx.Rollback(ctx) // does nothing after Commit; before it, the error already being returned is the one to report

	tag, err := tx.Exec(ctx, `
		INSERT INTO projects (id, github_id, owner, repo, description, created_by) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT DO NOTHING`,
		p.ID, p.GitHubID, p.Owner, p.Repo, p.Description, creatorID)
	if err != nil {
		return Project{}, false, unknownUserOr("create project", err)
	}
	if tag.RowsAffected() == 0 {
		// Read in this transaction: the pool is small, and a second connection per waiting
		// request could use it up.
		existing, found, err := projectOf(ctx, tx, p.GitHubID, p.Owner, p.Repo, creatorID)
		if err != nil {
			return Project{}, false, err
		}
		if !found {
			return Project{}, false, errors.New("store: create project: the repository was taken and is gone again")
		}
		return existing, false, nil
	}
	if _, err := tx.Exec(ctx, addMaintainerSQL, p.ID, creatorID); err != nil {
		return Project{}, false, unknownUserOr("create project", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Project{}, false, fmt.Errorf("store: create project: %w", err)
	}
	p.IsMember, p.IsMaintainer = true, true
	return p, true, nil
}

// AddMaintainer makes a user a maintainer of a project that exists, and a member of its room
// (plan decision 4: a second maintainer of the repository registers it again).
func (s *Store) AddMaintainer(ctx context.Context, projectID, userID string) error {
	if _, err := s.pool.Exec(ctx, addMaintainerSQL, projectID, userID); err != nil {
		return unknownUserOr("add maintainer", err)
	}
	return nil
}

// AddMember records that a user joined a project's room (FR-PROJ-4). Joining twice is one row.
func (s *Store) AddMember(ctx context.Context, projectID, userID string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO members (project_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, projectID, userID)
	if err != nil {
		return unknownUserOr("add member", err)
	}
	return nil
}

// Member is one person in a room, as the chat shows them.
type Member struct {
	UserID    string
	Name      string // never empty: the login stands in for a missing display name
	AvatarURL string
}

// maxMembersListed bounds one room's member list.
// ponytail: no paging. Nexconn caps a group well below this on the plans we use; page the list
// when a room can be larger.
const maxMembersListed = 3000

// Members lists who is in a project's room, in the order they joined.
func (s *Store) Members(ctx context.Context, projectID string) ([]Member, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, COALESCE(NULLIF(u.name, ''), u.login), u.avatar_url
		FROM members m
		JOIN users u ON u.id = m.user_id
		WHERE m.project_id = $1
		ORDER BY m.joined_at, u.id
		LIMIT $2`, projectID, maxMembersListed)
	if err != nil {
		return nil, fmt.Errorf("store: list members: %w", err)
	}
	members, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Member])
	if err != nil {
		return nil, fmt.Errorf("store: list members: %w", err)
	}
	return members, nil
}

// IsMember reports whether the user has joined the project's room. With IsMaintainer it is what
// the guard asks before a request that needs a role (NFR-SEC-2).
func (s *Store) IsMember(ctx context.Context, projectID, userID string) (bool, error) {
	return s.hasRow(ctx, `SELECT EXISTS (SELECT 1 FROM members WHERE project_id = $1 AND user_id = $2)`, projectID, userID)
}

// IsMaintainer reports whether the user maintains the project.
func (s *Store) IsMaintainer(ctx context.Context, projectID, userID string) (bool, error) {
	return s.hasRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintainers WHERE project_id = $1 AND user_id = $2)`, projectID, userID)
}

// Mute records when a member's mute in the project's room ends (FR-MOD-1); a later mute replaces
// an earlier one. Someone who is not a member stays as they are.
func (s *Store) Mute(ctx context.Context, projectID, userID string, until time.Time) error {
	if _, err := s.pool.Exec(ctx, `UPDATE members SET muted_until = $3 WHERE project_id = $1 AND user_id = $2`, projectID, userID, until); err != nil {
		return fmt.Errorf("store: mute: %w", err)
	}
	return nil
}

// MutedUntil is when the member's mute in the project's room ends: the zero time when they were
// never muted or are not a member.
func (s *Store) MutedUntil(ctx context.Context, projectID, userID string) (time.Time, error) {
	var until *time.Time // max of no rows is one NULL row
	if err := s.pool.QueryRow(ctx, `SELECT max(muted_until) FROM members WHERE project_id = $1 AND user_id = $2`, projectID, userID).Scan(&until); err != nil {
		return time.Time{}, fmt.Errorf("store: muted until: %w", err)
	}
	if until == nil {
		return time.Time{}, nil
	}
	return *until, nil
}

func (s *Store) hasRow(ctx context.Context, sql, projectID, userID string) (bool, error) {
	var has bool
	if err := s.pool.QueryRow(ctx, sql, projectID, userID).Scan(&has); err != nil {
		return false, fmt.Errorf("store: role lookup: %w", err)
	}
	return has, nil
}

// maxProjectsListed bounds the dashboard's list.
// ponytail: no paging. Add it when someone is in more rooms than this.
const maxProjectsListed = 100

// ProjectsOf lists the projects a user is a member of, by name. Maintainers are members too.
func (s *Store) ProjectsOf(ctx context.Context, userID string) ([]Project, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.github_id, p.owner, p.repo, p.description,
		       EXISTS (SELECT 1 FROM maintainers t WHERE t.project_id = p.id AND t.user_id = m.user_id)
		FROM members m
		JOIN projects p ON p.id = m.project_id
		WHERE m.user_id = $1
		ORDER BY lower(p.owner), lower(p.repo)
		LIMIT $2`, userID, maxProjectsListed)
	if err != nil {
		return nil, fmt.Errorf("store: list projects: %w", err)
	}
	projects, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Project, error) {
		p := Project{IsMember: true}
		err := row.Scan(&p.ID, &p.GitHubID, &p.Owner, &p.Repo, &p.Description, &p.IsMaintainer)
		return p, err
	})
	if err != nil {
		return nil, fmt.Errorf("store: list projects: %w", err)
	}
	return projects, nil
}

// unknownUserOr turns Postgres refusing a row for want of its user into ErrUnknownUser, and wraps
// every other error with what was being done.
func unknownUserOr(op string, err error) error {
	const foreignKeyViolation = "23503"
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation &&
		(strings.HasSuffix(pgErr.ConstraintName, "_user_id_fkey") || strings.HasSuffix(pgErr.ConstraintName, "_created_by_fkey")) {
		return ErrUnknownUser
	}
	return fmt.Errorf("store: %s: %w", op, err)
}
