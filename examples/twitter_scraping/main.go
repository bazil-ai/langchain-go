package main

import (
	"fmt"
	"github.com/go-rod/rod"
	"github.com/go-rod/stealth"
	"github.com/sashabaranov/go-openai"
	log "github.com/sirupsen/logrus"
	"gitlab.com/bazil/langchain-go/agents"
	"os"
	"time"
)

func main() {
	browser := rod.New().Timeout(time.Minute).MustConnect()
	defer browser.MustClose()

	oai := openai.NewClient(os.Getenv("OPENAI_API_KEY"))
	username := "laurentzeimes"
	page := stealth.MustPage(browser)
	logger := log.New()
	logger.Level = log.DebugLevel
	agent := agents.NewDomExplorer(oai).WithLogger(logger.WithField("agent", "dom-explorer"))
	page.MustNavigate(fmt.Sprintf("https://twitter.com/%s/with_replies", username))
	page.MustWaitIdle()
	page.MustElement("div[data-testid=\"UserDescription\"]")
	timeline := page.MustElement("div[aria-label=\"Home timeline\"] section")
	timelineHTML := timeline.MustHTML()
	page.MustWaitIdle()
	scrolling := true
	for scrolling {
		tweets, err := agent.GetElements(timeline, "The list of Elements containing the tweets in the timeline")
		if err != nil {
			panic(fmt.Errorf("error getting tweets: %w", err))
		}
		fmt.Println("got tweets", len(tweets))
		for _, tweet := range tweets {
			tweetText, err := agent.GetElement(tweet, "The Element with the tweet content only, without the date")
			if err != nil {
				logger.Errorf("error getting tweet text: %v", err)
				continue
			}
			tweetAuthor, err := agent.GetElement(tweet, "The Element which contains the twitter handle of the user")
			if err != nil {
				logger.Errorf("error getting tweet author: %v", err)
				continue
			}
			author, err := tweetAuthor.Evaluate(&rod.EvalOptions{JS: "() => { return this.textContent }", ThisObj: tweetAuthor.Object})
			if err != nil {
				logger.Errorf("error getting tweet author text: %v", err)
				continue
			}
			twt, err := tweetText.Evaluate(&rod.EvalOptions{JS: "() => { return this.textContent }", ThisObj: tweetText.Object})
			if err != nil {
				logger.Errorf("error getting tweet text: %v", err)
				continue
			}
			fmt.Println("TWEET", author.Value, twt.Value.Str())
		}
		// scroll down to load all tweets
		if len(tweets) == 0 {
			scrolling = false
		} else {
			tweets[len(tweets)-1].MustScrollIntoView()
			time.Sleep(1 * time.Second)
			page.MustWaitIdle()
			newTimeline := page.MustElement("div[aria-label=\"Home timeline\"] section")
			scrolling = newTimeline.MustHTML() != timelineHTML
			timelineHTML = newTimeline.MustHTML()
		}
	}
}
