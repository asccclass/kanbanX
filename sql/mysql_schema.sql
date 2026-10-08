-- KanbanX MySQL schema
-- Usage: mysql -u root -p < sql/mysql_schema.sql

CREATE DATABASE IF NOT EXISTS `kanbanx`
  CHARACTER SET utf8mb4
  COLLATE utf8mb4_unicode_ci;

USE `kanbanx`;

CREATE TABLE IF NOT EXISTS `boards` (
    `id`          VARCHAR(64)  NOT NULL,
    `telegram_id` VARCHAR(255) NOT NULL,
    `title`       VARCHAR(255) NOT NULL DEFAULT '我的看板',
    `created_at`  DATETIME     NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uq_boards_telegram_id` (`telegram_id`),
    KEY `idx_boards_telegram` (`telegram_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `columns` (
    `id`         VARCHAR(64)  NOT NULL,
    `board_id`   VARCHAR(64)  NOT NULL,
    `title`      VARCHAR(255) NOT NULL,
    `color`      VARCHAR(32)  NOT NULL DEFAULT '#6366f1',
    `position`   INT          NOT NULL DEFAULT 0,
    `created_at` DATETIME     NOT NULL,
    PRIMARY KEY (`id`),
    KEY `idx_columns_board` (`board_id`, `position`),
    CONSTRAINT `fk_columns_board`
      FOREIGN KEY (`board_id`) REFERENCES `boards` (`id`)
      ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `cards` (
    `id`          VARCHAR(64)  NOT NULL,
    `column_id`   VARCHAR(64)  NOT NULL,
    `title`       VARCHAR(255) NOT NULL,
    `description` TEXT         NOT NULL,
    `priority`    VARCHAR(16)  NOT NULL DEFAULT 'medium',
    `assignee`    VARCHAR(255) NOT NULL DEFAULT '',
    `labels`      JSON         NOT NULL,
    `position`    INT          NOT NULL DEFAULT 0,
    `created_at`  DATETIME     NOT NULL,
    `updated_at`  DATETIME     NOT NULL,
    PRIMARY KEY (`id`),
    KEY `idx_cards_column` (`column_id`, `position`),
    CONSTRAINT `fk_cards_column`
      FOREIGN KEY (`column_id`) REFERENCES `columns` (`id`)
      ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
