CREATE INDEX match_history_by_created ON match_rooms(created_at DESC, id DESC)
    WHERE status='closed' AND load_status='complete';
