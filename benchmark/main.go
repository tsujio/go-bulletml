package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tsujio/go-bulletml"
)

const loop = 10000

var testCases = map[string]string{
	"nop": `
	<bulletml>
		<action label="top">
			<repeat>
				<times>1000</times>
				<action>
					<fire>
						<bullet />
					</fire>
				</action>
			</repeat>
		</action>
	</bulletml>
	`,

	"repeat": `
	<bulletml>
		<action label="top">
			<repeat>
				<times>1000</times>
				<action>
					<fire>
						<bullet>
							<action>
								<repeat>
									<times>` + strconv.Itoa(loop) + `</times>
									<action>
										<wait>0</wait>
									</action>
								</repeat>
							</action>
						</bullet>
					</fire>
				</action>
			</repeat>
		</action>
	</bulletml>
	`,

	"fire": `
	<bulletml>
		<action label="top">
			<fire>
				<bullet>
					<action>
						<repeat>
							<times>` + strconv.Itoa(loop) + `</times>
							<action>
								<repeat>
									<times>100</times>
									<action>
										<fire>
											<bullet />
										</fire>
									</action>
								</repeat>
								<wait>0</wait>
							</action>
						</repeat>
					</action>
				</bullet>
			</fire>
		</action>
	</bulletml>
	`,
}

func main() {
	source, exists := testCases[os.Args[1]]
	if !exists {
		var tc []string
		for k, _ := range testCases {
			tc = append(tc, k)
		}
		panic("Please choose from: " + strings.Join(tc, ", "))
	}

	bml, err := bulletml.Load(bytes.NewReader([]byte(source)))
	if err != nil {
		panic(err)
	}

	var runners []bulletml.BulletRunner

	runner, err := bulletml.NewRunner(bml, &bulletml.NewRunnerOptions{
		OnBulletFired: func(bulletRunner bulletml.BulletRunner, _ *bulletml.FireContext) {
			runners = append(runners, bulletRunner)
		},
		CurrentShootPosition:  func() (float64, float64) { return 0, 0 },
		CurrentTargetPosition: func() (float64, float64) { return 0, 0 },
	})
	if err != nil {
		panic(err)
	}

	if err := runner.Update(); err != nil {
		panic(err)
	}

	_runners := runners[:]

	start := time.Now().UnixNano()

	for i := 0; i < loop; i++ {
		for _, r := range _runners {
			if err := r.Update(); err != nil {
				panic(err)
			}
		}
	}

	end := time.Now().UnixNano()

	json.NewEncoder(os.Stdout).Encode(map[string]any{
		"testCase":    os.Args[1],
		"bulletCount": len(_runners),
		"loopCount":   loop,
		"elapsedNano": end - start,
	})
}
