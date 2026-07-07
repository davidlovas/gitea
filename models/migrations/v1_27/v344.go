// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_27

import (
	"gitea.dev/models/db"

	"xorm.io/xorm"
)

// AddMetadataSyncOptionsToMirror adds the issue and pull-request sync toggles to
// the mirror table so a pull mirror can additionally keep issue/pull-request
// metadata current from its remote source. Both default off, preserving the
// existing git-only mirror behavior.
func AddMetadataSyncOptionsToMirror(x db.EngineMigration) error {
	type Mirror struct {
		SyncIssues       bool `xorm:"NOT NULL DEFAULT false"`
		SyncPullRequests bool `xorm:"NOT NULL DEFAULT false"`
	}

	_, err := x.SyncWithOptions(xorm.SyncOptions{
		IgnoreDropIndices: true,
		IgnoreConstrains:  true,
	}, new(Mirror))
	return err
}
