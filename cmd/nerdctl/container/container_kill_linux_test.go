/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package container

import (
	"fmt"
	"strings"
	"testing"

	"github.com/coreos/go-iptables/iptables"
	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	iptablesutil "github.com/containerd/nerdctl/v2/pkg/testutil/iptables"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

// TestKillCleanupForwards verifies that iptables port forwarding
// rules created by `nerdctl run -p` are removed after `nerdctl kill`.
func TestKillCleanupForwards(t *testing.T) {
	if rootlessutil.IsRootless() {
		t.Skip("iptables hostport rules are not supported in rootless mode")
	}

	const hostPort = 9999

	testCase := nerdtest.Setup()

	ipt, err := iptables.New()
	assert.NilError(t, err)

	// --------------------
	// Setup: run container
	// --------------------
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure(
			"run", "-d",
			"--restart=no",
			"--name", data.Identifier(),
			"-p", fmt.Sprintf("127.0.0.1:%d:80", hostPort),
			testutil.NginxAlpineImage,
		)
	}

	// --------------------
	// Cleanup: remove container
	// --------------------
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	// --------------------
	// Command: kill container
	// --------------------
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		name := data.Identifier()

		// inspect: kill 결과 검증을 위한 좌표 수집
		containerID := strings.TrimSpace(
			helpers.Capture("inspect", "-f", "{{.Id}}", name),
		)

		containerIP := strings.TrimSpace(
			helpers.Capture(
				"inspect",
				"-f", "{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}",
				name,
			),
		)

		// chain 결정
		var chain string
		if nerdtest.IsDocker() {
			chain = "DOCKER"
		} else {
			chain = iptablesutil.GetRedirectedChain(
				t,
				ipt,
				"CNI-HOSTPORT-DNAT",
				testutil.Namespace,
				containerID,
			)
		}

		// kill 전 전제 조건 검증
		assert.Assert(
			helpers.T(),
			iptablesutil.ForwardExists(t, ipt, chain, containerIP, hostPort),
			"expected iptables forward to exist before kill",
		)

		// 검증 대상 행위
		return helpers.Command("kill", name)
	}

	testCase.Expected = test.Expects(0, nil, nil)
	
	testCase.Run(t)
}
