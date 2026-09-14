-- Schema for gitlab-tg-notify. See PRD §9.
--
-- issue_threads maps one GitLab issue to the Telegram message that first
-- announced it, so later events (update/comment/close/reopen) can be sent
-- as a reply to that root message.

CREATE TABLE IF NOT EXISTS issue_threads (
    gitlab_project_id           INTEGER NOT NULL,
    gitlab_issue_iid            INTEGER NOT NULL,
    telegram_root_message_id    INTEGER NOT NULL,
    is_confidential              INTEGER NOT NULL DEFAULT 0,   -- 0 = false, 1 = true
    created_at                   TEXT    NOT NULL,             -- ISO 8601
    updated_at                   TEXT    NOT NULL,             -- ISO 8601, last time we wrote to this row
    PRIMARY KEY (gitlab_project_id, gitlab_issue_iid)
);

CREATE INDEX IF NOT EXISTS idx_issue_threads_project
    ON issue_threads (gitlab_project_id);

-- issue_time_tracking stores the last known cumulative "total time spent"
-- GitLab reported for an issue, so the bot can compute how much time was
-- logged by one specific comment (current total - last known total) instead
-- of showing the issue-wide running total on every message.

CREATE TABLE IF NOT EXISTS issue_time_tracking (
    gitlab_project_id   INTEGER NOT NULL,
    gitlab_issue_iid    INTEGER NOT NULL,
    last_total_secs     INTEGER NOT NULL DEFAULT 0,
    updated_at          TEXT    NOT NULL,  -- ISO 8601, last time we wrote to this row
    PRIMARY KEY (gitlab_project_id, gitlab_issue_iid)
);
