// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_27

import (
	"gitea.dev/models/db"

	"xorm.io/xorm"
)

// AddOriginalIDToCommentAndReview adds the original_id column to the comment and
// review tables, storing the entity id from the remote source. It lets an
// incremental mirror sync recognise already-imported comments and reviews and
// update them in place instead of duplicating them.
func AddOriginalIDToCommentAndReview(x db.EngineMigration) error {
	type Comment struct {
		OriginalID int64 `xorm:"INDEX"`
	}
	type Review struct {
		OriginalID int64 `xorm:"INDEX"`
	}

	_, err := x.SyncWithOptions(xorm.SyncOptions{
		IgnoreDropIndices: true,
		IgnoreConstrains:  true,
	}, new(Comment), new(Review))
	return err
}
