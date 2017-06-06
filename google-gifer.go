package googlegifer

import (
	"encoding/base64"
	"io/ioutil"
	"net/http"
	"regexp"
	"time"

	"fmt"
	"strings"

	sj "github.com/bitly/go-simplejson"
	"github.com/songtianyi/wechat-go/wxweb"
)

var regURL1, regURL2 *regexp.Regexp
var apiKey string
var reqJSON string

func init() {
	regURL1 = regexp.MustCompile(`cdnurl[[:blank:]]?=[[:blank:]]?"(http://emoji\.qpic\.cn/wx_emoji/[[:alnum:]]*/)"`)
	regURL2 = regexp.MustCompile(`cdnurl[[:blank:]]?=[[:blank:]]?"(http://mmbiz\.qpic\.cn/mmemoticon/[[:alnum:]]*/[[:digit:]])"`)
	apiKey = "Google Cloud Platform API Key"
	reqJSON = `{
  "requests": [
    {
      "image": {
        "content": "%s"
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
	session.HandlerRegister.Add(wxweb.MSG_LINK, wxweb.Handler(requestSimilarImages), "google-gifer_link")
}

func requestSimilarImages(session *wxweb.Session, msg *wxweb.ReceivedMessage) {
	to := wxweb.RealTargetUserName(session, msg)

	bytes := emotionBytes(session, msg)
	if len(bytes) == 0 {
		session.SendText("无法获取表情", session.Bot.UserName, to)
		return
	}

	var client = &http.Client{Timeout: time.Second * time.Duration(10)}
	resp, err := client.Post("https://vision.googleapis.com/v1/images:annotate?key="+apiKey, "application/json", strings.NewReader(fmt.Sprintf(reqJSON, base64.StdEncoding.EncodeToString(bytes))))
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
	count := len(arr.MustArray())
	// session.SendText(fmt.Sprintf("获取了%d个图片URL", count), session.Bot.UserName, to)
	ch := make(chan bool)
	go func() {
		for i := 0; i < count; i++ {
			ch <- true
			time.Sleep(time.Millisecond * 500)
		}
	}()
	for i := 0; i < count; i++ {
		go func(url string) {
			b := downloadImage(url)
			<-ch
			if len(b) > 0 {
				session.SendEmotionFromBytes(b, session.Bot.UserName, to)
			} else {
				// session.SendText("获取图片失败", session.Bot.UserName, to)
			}
		}(arr.GetIndex(i).Get("url").MustString())
	}
}

func emotionBytes(session *wxweb.Session, msg *wxweb.ReceivedMessage) (data []byte) {
	if msg.MsgType == wxweb.MSG_LINK {
		if len(msg.MsgId) > 0 {
			if b, err := session.GetImg(msg.MsgId); err == nil {
				data = b
			}
		}
	} else {
		url := extractCdnURL(msg.Content)
		if url != "" {
			data = downloadImage(url)
		}
	}
	return
}

func extractCdnURL(c string) string {
	m := regURL1.FindStringSubmatch(c)
	if len(m) > 1 {
		return m[1]
	}
	m = regURL2.FindStringSubmatch(c)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

func downloadImage(url string) (data []byte) {
	var client = &http.Client{Timeout: time.Second * time.Duration(5)}
	if resp, err := client.Get(url); err == nil {
		defer resp.Body.Close()
		data, _ = ioutil.ReadAll(resp.Body)
	}
	return
}
