package lats_test

import (
	"crypto/tls"
	"encoding/json"
	"github.com/cloudfoundry-incubator/cf-test-helpers/cf"
	"github.com/cloudfoundry-incubator/cf-test-helpers/generator"
	"github.com/cloudfoundry-incubator/cf-test-helpers/helpers"
	"github.com/cloudfoundry/noaa"
	"github.com/cloudfoundry/sonde-go/events"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"regexp"
	"strings"
	"time"
)

var _ = Describe("Streaming logs from an app", func() {
	var appName string
	var authToken string

	BeforeEach(func() {
		appName = pushApp()
		authToken = fetchOAuthToken()
	})

	It("succeeds in sending all log lines", func(done Done) {
		msgChan := make(chan *events.LogMessage)
		errorChan := make(chan error, 5)
		connection := noaa.NewConsumer(getDopplerEndpoint(), &tls.Config{InsecureSkipVerify: config.SkipSSLValidation}, nil)
		defer connection.Close()

		appGuid := getAppGuid(appName)
		go connection.TailingLogs(appGuid, authToken, msgChan, errorChan)

		// Make app log some logs
		helpers.CurlApp(appName, "/loglines/5/LogStreamTestMarker")

		// Expect all logs to appear in Noaa consumer
		messages := waitForLogMessages(5, msgChan, errorChan)
		Expect(messages).To(HaveLen(5))

		for _, message := range messages {
			Expect(message.GetAppId()).To(Equal(appGuid))
			Expect(string(message.GetMessage())).To(ContainSubstring("LogStreamTestMarker"))
		}

		Consistently(errorChan).ShouldNot(Receive())

		close(done)
	}, 120)
})

type apiInfo struct {
	DopplerLoggingEndpoint string `json:"doppler_logging_endpoint"`
}

func getDopplerEndpoint() string {
	info := apiInfo{}
	ccInfo := cf.Cf("curl", "/v2/info").Wait(5 * time.Second)
	json.Unmarshal(ccInfo.Out.Contents(), &info)
	return info.DopplerLoggingEndpoint
}

func getAppGuid(appName string) string {
	appGuid := cf.Cf("app", appName, "--guid").Wait(5 * time.Second).Out.Contents()
	return strings.TrimSpace(string(appGuid))
}

func waitForLogMessages(maxMessages int, msgChan chan *events.LogMessage, errorChan chan error) []*events.LogMessage {
	messages := make([]*events.LogMessage, 0, maxMessages)

	for msg := range msgChan {
		if len(messages) == maxMessages {
			return messages
		}
		messages = append(messages, msg)
	}
	return messages
}

func pushApp() string {
	appName := generator.PrefixedRandomName("LATS-App-")
	appPush := cf.Cf("push", appName, "-p", "assets/dora").Wait(60 * time.Second)
	Expect(appPush).To(gexec.Exit(0))

	return appName
}

func fetchOAuthToken() string {
	reg := regexp.MustCompile(`(bearer.*)`)
	output := string(cf.Cf("oauth-token").Wait().Out.Contents())
	authToken := reg.FindString(output)
	return authToken
}
