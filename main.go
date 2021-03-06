package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"

	_ "github.com/heroku/x/hmetrics/onload"
	"github.com/nlopes/slack"
	"github.com/nlopes/slack/slackevents"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		log.Fatal("$PORT must be set")
	}

	oauthToken := os.Getenv("SLACK_OAUTH_ACCESS_TOKEN")
	if oauthToken == "" {
		log.Fatal("$SLACK_OAUTH_ACCESS_TOKEN must be set")
	}

	signedSecret := os.Getenv("SLACK_SIGNED_SECRET")
	if signedSecret == "" {
		log.Fatal("$SLACK_SIGNED_SECRET must be set")
	}

	http.HandleFunc("/events-endpoint", hundleEvent(oauthToken, signedSecret))

	log.Println("[INFO] Server listening")
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func hundleEvent(oauthToken string, signedSecret string) func(w http.ResponseWriter, r *http.Request) {
	var api = slack.New(oauthToken)

	return func(w http.ResponseWriter, r *http.Request) {
		buf := new(bytes.Buffer)
		if _, err := buf.ReadFrom(r.Body); err != nil {
			log.Fatal("Cannot read body: ", err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		body := buf.String()

		eventsAPIEvent, err := slackevents.ParseEvent(json.RawMessage(body), slackevents.OptionNoVerifyToken())
		if err != nil {
			log.Fatal("Cannot parse event: ", err)
			w.WriteHeader(http.StatusInternalServerError)
		}

		sv, err := slack.NewSecretsVerifier(r.Header, signedSecret)
		if err != nil {
			log.Fatal("Cannot NewSecretsVerifier: ", err)
			w.WriteHeader(http.StatusInternalServerError)
		}

		if _, err := sv.Write([]byte(body)); err != nil {
			log.Fatal("Cannot SecretsVerifier#Write: ", err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		if err := sv.Ensure(); err != nil {
			log.Fatal("Cannot Ensure signed secrets: ", err)
			w.WriteHeader(http.StatusInternalServerError)
		}

		if eventsAPIEvent.Type == slackevents.URLVerification {
			var r *slackevents.ChallengeResponse
			err := json.Unmarshal([]byte(body), &r)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
			}

			w.Header().Set("Content-Type", "text")
			if _, err := w.Write([]byte(r.Challenge)); err != nil {
				log.Fatal("Cannot make ChallengeResponse: ", err)
				w.WriteHeader(http.StatusInternalServerError)
			}
		}

		if eventsAPIEvent.Type == slackevents.CallbackEvent {
			innerEvent := eventsAPIEvent.InnerEvent

			switch ev := innerEvent.Data.(type) {
			case *slackevents.MessageEvent:
				if err := replyKeepaURL(api, ev); err != nil {
					log.Fatal("Cannot replyKeepaURL: ", err)
					w.WriteHeader(http.StatusInternalServerError)
				}
			}
		}
	}
}

func replyKeepaURL(api *slack.Client, ev *slackevents.MessageEvent) error {
	re := regexp.MustCompile(`amazon\.co\.jp.+?dp/([^/?]+)(\?|\/|$)`)

	log.Printf("%+v", ev.Text)

	for _, m := range re.FindAllStringSubmatch(ev.Text, -1) {
		if len(m) < 2 {
			continue
		}

		asin := m[1]
		log.Printf("asin = %s", asin)
		text := fmt.Sprintf("https://graph.keepa.com/pricehistory.png?domain=co.jp&asin=%s", asin)

		if _, _, err := api.PostMessage(ev.Channel, slack.MsgOptionText(text, false)); err != nil {
			log.Fatal("Cannot PostMessage: ", err)
			return err
		}
	}

	return nil
}
