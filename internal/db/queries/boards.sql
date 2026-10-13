-- name: CreateBoard :one
INSERT INTO boards (id, user_id, title)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListBoards :many
SELECT id, title, revision, created_at, updated_at
FROM boards
WHERE user_id = $1
ORDER BY updated_at DESC
LIMIT $2 OFFSET $3;

-- name: GetBoard :one
SELECT *
FROM boards
WHERE id = $1 AND user_id = $2;

-- name: SaveBoard :one
UPDATE boards
SET snapshot = $3::jsonb,
    snapshot_version = $4,
    doc_md = $5,
    revision = revision + 1,
    updated_at = now()
WHERE id = $1 AND user_id = $2 AND revision = $6
RETURNING *;

-- name: RenameBoard :one
UPDATE boards
SET title = $3, updated_at = now()
WHERE id = $1 AND user_id = $2
RETURNING *;

-- name: DeleteBoard :execrows
DELETE FROM boards
WHERE id = $1 AND user_id = $2;
