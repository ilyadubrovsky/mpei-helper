-- +goose Up
-- +goose StatementBegin
CREATE TABLE users (
    id BIGINT PRIMARY KEY
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS users;
-- +goose StatementEnd
