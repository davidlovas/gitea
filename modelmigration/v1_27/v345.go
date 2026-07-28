// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_27

import (
	"gitea.dev/modelmigration/base"

	"xorm.io/xorm"
)

func AddHeadRepoOwnerToPullRequest(x base.EngineMigration) error {
	type PullRequest struct {
		HeadRepoOwner string `xorm:"NOT NULL DEFAULT ''"`
	}
	_, err := x.SyncWithOptions(xorm.SyncOptions{
		IgnoreDropIndices: true,
		IgnoreConstrains:  true,
	}, new(PullRequest))
	return err
}
