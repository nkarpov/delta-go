// Copyright 2023 Rivian Automotive, Inc.
// Licensed under the Apache License, Version 2.0 (the “License”);
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an “AS IS” BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package state

import (
	"errors"
)

// / Type alias for i64/Delta long
type DeltaDataTypeLong int64

// / Type alias representing the expected type (i64) of a Delta table version.
type DeltaDataTypeVersion DeltaDataTypeLong

var (
	ErrorStateIsEmpty     error = errors.New("the state is empty")
	ErrorCanNotReadState  error = errors.New("the state is could not be read")
	ErrorCanNotWriteState error = errors.New("the state is could not be written")
)

// CommitState stores an attempt to  `source` into `destination` and `version` for the latest commit.
type CommitState struct {
	// Version of the commit
	Version DeltaDataTypeVersion `json:"version"`
}

// StateStore provides remote state storage for fast lookup on the current commit version
type StateStore interface {
	// GetData() retrieves the data cached in the lock.
	// for a DeltaTable, the data will contain the current or prior locked commit version.
	Get() (CommitState, error)

	// GetData() retrieves the data cached in the lock.
	// for a DeltaTable, the data will contain the current or prior locked commit version.
	Put(CommitState) error
}
