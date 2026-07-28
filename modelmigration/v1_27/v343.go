// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_27

import (
	"gitea.dev/modelmigration/base"

	"xorm.io/xorm"
)

func AddOriginalIDToCommentAndReview(x base.EngineMigration) error {
	type Comment struct {
		OriginalID int64 `xorm:"index"`
	}
	type Review struct {
		OriginalID int64 `xorm:"index"`
	}
	_, err := x.SyncWithOptions(xorm.SyncOptions{
		IgnoreDropIndices: true,
		IgnoreConstrains:  true,
	}, new(Comment), new(Review))
	return err
}
