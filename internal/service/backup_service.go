package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"prompt736/internal/models"
	"prompt736/internal/types"
	"prompt736/internal/utils"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GetBackupList 获取备份文件列表
func (app *App) GetBackupList() ([]types.BackupInfo, error) {
	var backups []types.BackupInfo
	if err := os.MkdirAll(app.Cfg.BackupDir, 0755); err != nil {
		app.Log.WithError(err).Warn("create backup dir failed")
	}

	files, err := os.ReadDir(app.Cfg.BackupDir)
	if err != nil {
		return nil, err
	}

	for _, f := range files {
		if f.IsDir() {
			continue
		}
		filename := strings.ToLower(f.Name())
		if !strings.HasSuffix(filename, ".sql") && !strings.HasSuffix(filename, ".json") {
			continue
		}
		info, infoErr := f.Info()
		if infoErr != nil {
			continue
		}
		backups = append(backups, types.BackupInfo{
			Name: f.Name(),
			Time: info.ModTime().Format("2006-01-02 15:04"),
		})
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Name > backups[j].Name
	})
	return backups, nil
}

// CollectBackupSnapshot 收集备份快照
func (app *App) CollectBackupSnapshot() (*types.BackupSnapshot, error) {
	snapshot := &types.BackupSnapshot{
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
func writeSnapshotAtomic(path string, snapshot *types.BackupSnapshot) error {
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

// CreateBackup 创建备份
func (app *App) CreateBackup() (string, error) {
	if err := os.MkdirAll(app.Cfg.BackupDir, 0755); err != nil {
		return "", err
	}

	snapshot, err := app.CollectBackupSnapshot()
	if err != nil {
		return "", err
	}

	filename := fmt.Sprintf("backup_%s.json", time.Now().Format("20060102_150405"))
	backupPath := filepath.Join(app.Cfg.BackupDir, filename)
	if err := writeSnapshotAtomic(backupPath, snapshot); err != nil {
		return "", err
	}

	return filename, nil
}

// resolveBackupPath 解析备份文件路径（安全检查）
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

// createRows 批量创建行
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

// RestoreFromJSONBackup 从JSON备份恢复
func (app *App) RestoreFromJSONBackup(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var snapshot types.BackupSnapshot
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

// RestoreFromSQLBackup 从SQL备份恢复
func (app *App) RestoreFromSQLBackup(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	statements := utils.SplitSQLStatements(string(raw))
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

// RestoreBackup 恢复备份
func (app *App) RestoreBackup(filename string) error {
	backupPath, err := app.resolveBackupPath(filename)
	if err != nil {
		return err
	}

	switch strings.ToLower(filepath.Ext(backupPath)) {
	case ".json":
		err = app.RestoreFromJSONBackup(backupPath)
	case ".sql":
		err = app.RestoreFromSQLBackup(backupPath)
	default:
		err = fmt.Errorf("unsupported backup extension: %s", filepath.Ext(backupPath))
	}

	if err != nil {
		return err
	}

	app.ClearCache()
	return nil
}
