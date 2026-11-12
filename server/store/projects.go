package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

// ProjectByRepo finds the project of a repository, in any letter case (FR-PROJ-2), and says what
// viewerID is in it. An empty viewerID is a signed-out visitor.
func (s *Store) ProjectByRepo(ctx context.Context, owner, repo, viewerID string) (Project, bool, error) {
	return projectByRepo(ctx, s.pool, owner, repo, viewerID)
}

func projectByRepo(ctx context.Context, q rowQuerier, owner, repo, viewerID string) (Project, bool, error) {
	var p Project
	err := q.QueryRow(ctx, `
		SELECT p.id, p.owner, p.repo, p.description,
		       EXISTS (SELECT 1 FROM members m WHERE m.project_id = p.id AND m.user_id = $3),
		       EXISTS (SELECT 1 FROM maintainers t WHERE t.project_id = p.id AND t.user_id = $3)
		FROM projects p
		WHERE lower(p.owner) = lower($1::text) AND lower(p.repo) = lower($2::text)`,
		owner, repo, viewerID).Scan(&p.ID, &p.Owner, &p.Repo, &p.Description, &p.IsMember, &p.IsMaintainer)
	if errors.Is(err, pgx.ErrNoRows) {
		return Project{}, false, nil
	}
	if err != nil {
		return Project{}, false, fmt.Errorf("store: find project: %w", err)
	}
	return p, true, nil
}

// addMaintainerSQL makes a user a maintainer and a member in one statement, so both rows exist or
// neither. Someone who is one or both already stays as they are.
const addMaintainerSQL = `
	WITH added AS (
		INSERT INTO maintainers (project_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING
	)
	INSERT INTO members (project_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`

// CreateProject records a new project with creatorID as its first maintainer and member
// (FR-PROJ-2). If the repository is registered already it writes nothing and returns that project
// as creatorID sees it, with created false. Of two registrations at the same moment one wins,
// and the other learns the winner's project: the unique index on the repository's name decides.
func (s *Store) CreateProject(ctx context.Context, p Project, creatorID string) (project Project, created bool, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Project{}, false, fmt.Errorf("store: create project: %w", err)
	}
	defer tx.Rollback(ctx) // does nothing after Commit; before it, the error already being returned is the one to report

	tag, err := tx.Exec(ctx, `
		INSERT INTO projects (id, owner, repo, description, created_by) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (lower(owner), lower(repo)) DO NOTHING`,
		p.ID, p.Owner, p.Repo, p.Description, creatorID)
	if err != nil {
		return Project{}, false, unknownUserOr("create project", err)
	}
	if tag.RowsAffected() == 0 {
		// Read in this transaction: the pool is small, and a second connection per waiting
		// request could use it up.
		existing, found, err := projectByRepo(ctx, tx, p.Owner, p.Repo, creatorID)
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

// maxProjectsListed bounds the dashboard's list.
// ponytail: no paging. Add it when someone is in more rooms than this.
const maxProjectsListed = 100

// ProjectsOf lists the projects a user is a member of, by name. Maintainers are members too.
func (s *Store) ProjectsOf(ctx context.Context, userID string) ([]Project, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.owner, p.repo, p.description,
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
		err := row.Scan(&p.ID, &p.Owner, &p.Repo, &p.Description, &p.IsMaintainer)
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
