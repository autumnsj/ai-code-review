-- 0005: 审查执行过程日志（实时进度，前端轮询展示）(MySQL)

CREATE TABLE IF NOT EXISTS review_logs (
    id         BIGINT AUTO_INCREMENT PRIMARY KEY,
    review_id  BIGINT NOT NULL,
    level      VARCHAR(16) NOT NULL DEFAULT 'info',  -- info|warn|error
    message    TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_review_logs_review FOREIGN KEY (review_id) REFERENCES reviews(id) ON DELETE CASCADE,
    INDEX idx_review_logs_review (review_id, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
