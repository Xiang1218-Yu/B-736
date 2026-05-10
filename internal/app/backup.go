package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AdminBackup 创建数据库备份
func (app *App) AdminBackup(c *gin.Context) {
	if err := os.MkdirAll(app.Cfg.BackupDir, 0755); err != nil {
		app.Log.WithError(err).Error("create backup dir failed")
		app.setFlashKey(c, "flash_error", "flash.backup_failed")
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	snapshot, err := app.collectBackupSnapshot()
	if err != nil {
		app.Log.WithError(err).Error("collect backup snapshot failed")
		app.setFlashKey(c, "flash_error", "flash.backup_failed")
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	filename := fmt.Sprintf("backup_%s.json", time.Now().Format("20060102_150405"))
	backupPath := filepath.Join(app.Cfg.BackupDir, filename)
	if err := writeSnapshotAtomic(backupPath, snapshot); err != nil {
		app.Log.WithError(err).Error("write backup file failed")
		app.setFlashKey(c, "flash_error", "flash.backup_failed")
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	app.setFlashKey(c, "flash_success", "flash.backup_success")
	c.Redirect(http.StatusFound, "/admin")
}

// AdminRestore 从备份恢复数据库
func (app *App) AdminRestore(c *gin.Context) {
	filename := c.PostForm("filename")
	if filename == "" {
		app.setFlashKey(c, "flash_error", "flash.select_backup")
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	backupPath, err := app.resolveBackupPath(filename)
	if err != nil {
		app.Log.WithError(err).WithField("filename", filename).Warn("invalid backup file path")
		app.setFlashKey(c, "flash_error", "flash.invalid_backup_file")
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	switch strings.ToLower(filepath.Ext(backupPath)) {
	case ".json":
		err = app.restoreFromJSONBackup(backupPath)
	case ".sql":
		err = app.restoreFromSQLBackup(backupPath)
	default:
		err = fmt.Errorf("unsupported backup extension: %s", filepath.Ext(backupPath))
	}
	if err != nil {
		app.Log.WithError(err).WithField("backup", backupPath).Error("restore backup failed")
		app.setFlashKey(c, "flash_error", "flash.restore_failed")
		c.Redirect(http.StatusFound, "/admin")
		return
	}

	app.Cache.Flush()
	app.setFlashKey(c, "flash_success", "flash.restore_success")
	c.Redirect(http.StatusFound, "/admin")
}

// collectBackupSnapshot 收集数据库备份快照
func (app *App) collectBackupSnapshot() (*BackupSnapshot, error) {
	snapshot := &BackupSnapshot{
		Version:     1,
		GeneratedAt: time.Now().UTC(),
	}

	if err := app.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Order("id asc").Find(&snapshot.Users).Error; err != nil {
			return err
		}
		if err := tx.Order("id asc").Find(&snapshot.Categories).Error; err != nil {
			return err
		}
		if err := tx.Order("id asc").Find(&snapshot.Tags).Error; err != nil {
			return err
		}
		if err := tx.Omit(clause.Associations).Order("id asc").Find(&snapshot.Resources).Error; err != nil {
			return err
		}
		if err := tx.Omit(clause.Associations).Order("id asc").Find(&snapshot.Articles).Error; err != nil {
			return err
		}
		if err := tx.Omit(clause.Associations).Order("id asc").Find(&snapshot.Comments).Error; err != nil {
			return err
		}
		if err := tx.Order("id asc").Find(&snapshot.SiteConfigs).Error; err != nil {
			return err
		}
		if err := tx.Order("id asc").Find(&snapshot.ShareLogs).Error; err != nil {
			return err
		}
		if err := tx.Order("resource_id asc, tag_id asc").Find(&snapshot.ResourceTags).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return snapshot, nil
}

// writeSnapshotAtomic 原子写入快照文件
func writeSnapshotAtomic(path string, snapshot *BackupSnapshot) error {
	payload, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, payload, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return nil
}

// resolveBackupPath 解析并验证备份文件路径
func (app *App) resolveBackupPath(filename string) (string, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return "", errors.New("filename is empty")
	}
	if filename != filepath.Base(filename) {
		return "", fmt.Errorf("invalid filename: %s", filename)
	}
	ext := strings.ToLower(filepath.Ext(filename))
	if ext != ".json" && ext != ".sql" {
		return "", fmt.Errorf("unsupported backup extension: %s", ext)
	}

	baseAbs, err := filepath.Abs(app.Cfg.BackupDir)
	if err != nil {
		return "", err
	}
	targetAbs, err := filepath.Abs(filepath.Join(baseAbs, filename))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(baseAbs, targetAbs)
	if err != nil {
		return "", err
	}
	if rel == "." || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("invalid backup file path: %s", filename)
	}
	info, err := os.Stat(targetAbs)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("backup file is directory: %s", filename)
	}

	return targetAbs, nil
}

// restoreFromJSONBackup 从 JSON 备份恢复数据库
func (app *App) restoreFromJSONBackup(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var snapshot BackupSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return err
	}

	return app.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET FOREIGN_KEY_CHECKS = 0").Error; err != nil {
			return err
		}
		defer func() {
			if enableErr := tx.Exec("SET FOREIGN_KEY_CHECKS = 1").Error; enableErr != nil {
				app.Log.WithError(enableErr).Warn("re-enable foreign key checks failed")
			}
		}()

		for _, table := range []string{
			"resource_tags",
			"share_logs",
			"comments",
			"articles",
			"resources",
			"tags",
			"categories",
			"users",
			"site_configs",
		} {
			if err := tx.Exec("DELETE FROM `" + table + "`").Error; err != nil {
				return err
			}
		}

		if err := createRows(tx, "users", snapshot.Users, false); err != nil {
			return err
		}
		if err := createRows(tx, "categories", snapshot.Categories, false); err != nil {
			return err
		}
		if err := createRows(tx, "tags", snapshot.Tags, false); err != nil {
			return err
		}
		if err := createRows(tx, "resources", snapshot.Resources, true); err != nil {
			return err
		}
		if err := createRows(tx, "articles", snapshot.Articles, true); err != nil {
			return err
		}
		if err := createRows(tx, "comments", snapshot.Comments, true); err != nil {
			return err
		}
		if err := createRows(tx, "site_configs", snapshot.SiteConfigs, false); err != nil {
			return err
		}
		if err := createRows(tx, "share_logs", snapshot.ShareLogs, false); err != nil {
			return err
		}
		if err := createRows(tx, "resource_tags", snapshot.ResourceTags, false); err != nil {
			return err
		}

		if err := tx.Exec("SET FOREIGN_KEY_CHECKS = 1").Error; err != nil {
			return err
		}
		return nil
	})
}

// restoreFromSQLBackup 从 SQL 备份恢复数据库
func (app *App) restoreFromSQLBackup(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	statements := splitSQLStatements(string(raw))
	if len(statements) == 0 {
		return errors.New("no executable SQL statements found")
	}

	return app.DB.Transaction(func(tx *gorm.DB) error {
		for _, statement := range statements {
			normalized := strings.TrimSpace(statement)
			if normalized == "" {
				continue
			}
			upper := strings.ToUpper(normalized)
			if strings.HasPrefix(upper, "DELIMITER ") {
				continue
			}
			if err := tx.Exec(normalized).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// createRows 批量创建行数据
func createRows[T any](tx *gorm.DB, table string, rows []T, omitAssociations bool) error {
	if len(rows) == 0 {
		return nil
	}
	db := tx.Table(table)
	if omitAssociations {
		db = db.Omit(clause.Associations)
	}
	return db.CreateInBatches(rows, 200).Error
}

// splitSQLStatements 分割 SQL 语句
func splitSQLStatements(sqlText string) []string {
	statements := make([]string, 0)
	var current strings.Builder

	inSingleQuote := false
	inDoubleQuote := false
	inBacktick := false
	inLineComment := false
	inBlockComment := false
	escaped := false

	runes := []rune(sqlText)
	for i := 0; i < len(runes); i++ {
		ch := runes[i]
		next := rune(0)
		if i+1 < len(runes) {
			next = runes[i+1]
		}

		if inLineComment {
			if ch == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if ch == '*' && next == '/' {
				inBlockComment = false
				i++
			}
			continue
		}

		if !inSingleQuote && !inDoubleQuote && !inBacktick {
			if ch == '-' && next == '-' {
				after := rune(0)
				if i+2 < len(runes) {
					after = runes[i+2]
				}
				if after == 0 || after == ' ' || after == '\t' || after == '\n' || after == '\r' {
					inLineComment = true
					i++
					continue
				}
			}
			if ch == '#' {
				inLineComment = true
				continue
			}
			if ch == '/' && next == '*' {
				inBlockComment = true
				i++
				continue
			}
		}

		if ch == '\'' && !inDoubleQuote && !inBacktick && !escaped {
			inSingleQuote = !inSingleQuote
		} else if ch == '"' && !inSingleQuote && !inBacktick && !escaped {
			inDoubleQuote = !inDoubleQuote
		} else if ch == '`' && !inSingleQuote && !inDoubleQuote {
			inBacktick = !inBacktick
		}

		if ch == ';' && !inSingleQuote && !inDoubleQuote && !inBacktick {
			statement := strings.TrimSpace(current.String())
			if statement != "" {
				statements = append(statements, statement)
			}
			current.Reset()
			escaped = false
			continue
		}

		current.WriteRune(ch)

		if (inSingleQuote || inDoubleQuote) && ch == '\\' && !escaped {
			escaped = true
		} else {
			escaped = false
		}
	}

	statement := strings.TrimSpace(current.String())
	if statement != "" {
		statements = append(statements, statement)
	}

	return statements
}
