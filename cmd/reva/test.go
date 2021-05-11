// Copyright 2018-2021 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package main

import (
	"fmt"
	"io"
	"strconv"
	"time"
)

var testCommand = func() *command {
	cmd := newCommand("test")
	cmd.Description = func() string { return "little test for a private new command" }
	cmd.Action = func(w ...io.Writer) error {

		start := time.Now()

		b, err := executeCommand(mkdirCommand(), "/home/testperf")

		if err != nil {
			fmt.Println("testtest ", b, err)
			return nil
		}

		elapsedmkdir := time.Since(start)

		start = time.Now()

		for i := 0; i < 500; i++ {

			b, err := executeCommand(uploadCommand(), "-protocol", "simple", "/tmp/1MFile", "/home/testperf/file-"+strconv.FormatInt(int64(i), 10))

			if err != nil {
				fmt.Println("testtest ", b, err)
				return nil
			}

			b.Reset()
		}

		elapsedupload := time.Since(start)

		start = time.Now()

		for i := 0; i < 500; i++ {

			b, err := executeCommand(downloadCommand(), "/home/testperf/file-"+strconv.FormatInt(int64(i), 10), "/tmp/1Mdeleteme")

			if err != nil {
				fmt.Println("testtest ", b, err)
				return nil
			}

			b.Reset()
		}

		elapseddownload := time.Since(start)

		start = time.Now()

		for i := 0; i < 500; i++ {

			b, err := executeCommand(rmCommand(), "/home/testperf/file-"+strconv.FormatInt(int64(i), 10))

			if err != nil {
				fmt.Println("testtest ", b, err)
				return nil
			}

			b.Reset()
		}

		elapsedrm := time.Since(start)

		fmt.Printf("mkdir took %s \n", elapsedmkdir)
		fmt.Printf("upload took %s \n", elapsedupload)
		fmt.Printf("download took %s \n", elapseddownload)
		fmt.Printf("rm took %s \n", elapsedrm)

		return nil
	}
	return cmd
}
