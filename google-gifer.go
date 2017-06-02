package main

import (
	"io/ioutil"
	"net/http"
	"regexp"
	"time"

	"fmt"
	"strings"

	sj "github.com/bitly/go-simplejson"
	"github.com/songtianyi/wechat-go/wxweb"
)

var regURL *regexp.Regexp
var apiKey string
var reqJSON string

func init() {
	regURL = regexp.MustCompile(`cdnurl[[:blank:]]?=[[:blank:]]?"(http://emoji\.qpic\.cn/wx_emoji/[[:alnum:]]*/)"`)
	apiKey = "Google Cloud Platform API Key"
	reqJSON = `{
  "requests": [
    {
      "image": {
        "source": {
          "imageUri": "%s"
        }
      },
      "features": [
        {
          "type": "WEB_DETECTION"
        }
      ]
    }
  ]
}`
}

// Register : register this plugin
func Register(session *wxweb.Session) {
	session.HandlerRegister.Add(wxweb.MSG_EMOTION, wxweb.Handler(requestSimilarImages), "google-gifer")
	// session.HandlerRegister.Add(wxweb.MSG_LINK, wxweb.Handler(echo), "echoemotion")
}

func requestSimilarImages(session *wxweb.Session, msg *wxweb.ReceivedMessage) {
	to := wxweb.RealTargetUserName(session, msg)

	url := extractCdnURL(msg.Content)
	if url == "" {
		session.SendText("无法获取表情", session.Bot.UserName, to)
		return
	}

	var client = &http.Client{Timeout: time.Second * time.Duration(10)}
	resp, err := client.Post("https://vision.googleapis.com/v1/images:annotate?key="+apiKey, "application/json", strings.NewReader(fmt.Sprintf(reqJSON, url)))
	if err != nil {
		session.SendText("访问Google Cloud Vision API失败", session.Bot.UserName, to)
		return
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	web, err := sj.NewJson(body)
	if err != nil {
		session.SendText("解析Google Cloud Vision API响应错误", session.Bot.UserName, to)
		return
	}
	if err, ok := web.CheckGet("error"); ok {
		session.SendText(err.Get("message").MustString("Unexpected Google Cloud Vision API response"), session.Bot.UserName, to)
		return
	}
	arr := web.Get("responses").GetIndex(0).Get("webDetection").Get("visuallySimilarImages")
	for i := range arr.MustArray() {
		go responseEmotion(session, to, arr.GetIndex(i).Get("url").MustString())
	}
}

func extractCdnURL(c string) string {
	m := regURL.FindStringSubmatch(c)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

func responseEmotion(session *wxweb.Session, to string, url string) {
	var client = &http.Client{Timeout: time.Second * time.Duration(5)}
	resp, err := client.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	session.SendEmotionFromBytes(body, session.Bot.UserName, to)
	// session.SendImgFromBytes(body, url, session.Bot.UserName, to)
}
