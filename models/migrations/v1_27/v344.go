// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_27

import (
	"gitea.dev/models/db"

	"xorm.io/xorm"
)

// AddMetadataSyncOptionsToMirror adds the per-entity sync toggles to the mirror
// table so a pull mirror can additionally keep issues, pull requests, comments,
// reviews, labels, milestones, releases and the wiki current from its remote
// source. All default off, preserving the existing git-only mirror behavior.
func AddMetadataSyncOptionsToMirror(x db.EngineMigration) error {
	type Mirror struct {
		SyncWiki         bool `xorm:"NOT NULL DEFAULT false"`
		SyncIssues       bool `xorm:"NOT NULL DEFAULT false"`
		SyncMilestones   bool `xorm:"NOT NULL DEFAULT false"`
		SyncLabels       bool `xorm:"NOT NULL DEFAULT false"`
		SyncReleases     bool `xorm:"NOT NULL DEFAULT false"`
		SyncComments     bool `xorm:"NOT NULL DEFAULT false"`
		SyncPullRequests bool `xorm:"NOT NULL DEFAULT false"`
	}

	_, err := x.SyncWithOptions(xorm.SyncOptions{
		IgnoreDropIndices: true,
		IgnoreConstrains:  true,
	}, new(Mirror))
	return err
}
