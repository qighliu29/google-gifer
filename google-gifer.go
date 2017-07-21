package googlegifer

import (
	"encoding/base64"
	"errors"
	"io/ioutil"
	"net/http"
	"regexp"
	"time"

	"fmt"
	"strings"

	sj "github.com/bitly/go-simplejson"
	"github.com/qighliu29/wechat-go/wxweb"
	"github.com/songtianyi/rrframework/logs"
)

var regURL1, regURL2 *regexp.Regexp
var reqJSON string

func init() {
	regURL1 = regexp.MustCompile(`cdnurl[[:blank:]]?=[[:blank:]]?"(http://emoji\.qpic\.cn/wx_emoji/[[:alnum:]]*/)"`)
	regURL2 = regexp.MustCompile(`cdnurl[[:blank:]]?=[[:blank:]]?"(http://mmbiz\.qpic\.cn/mmemoticon/[[:alnum:]]*/[[:digit:]])"`)
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

func requestSimilarImages(i interface{}, session *wxweb.Session, msg *wxweb.ReceivedMessage) {
	var (
		g  *Ggifer
		ok bool
	)
	if g, ok = i.(*Ggifer); !ok {
		logs.Error("Unexpected Ggifer pointer")
		return
	}

	// request filter
	if !g.reqFlt(session, msg) {
		return
	}

	to := wxweb.RealTargetUserName(session, msg)

	bytes := emotionBytes(session, msg)
	if len(bytes) == 0 {
		session.SendText("无法获取表情", session.Bot.UserName, to)
		return
	}

	var client = &http.Client{Timeout: time.Second * time.Duration(10)}
	resp, err := client.Post("https://vision.googleapis.com/v1/images:annotate?key="+g.apiKey, "application/json", strings.NewReader(fmt.Sprintf(reqJSON, base64.StdEncoding.EncodeToString(bytes))))
	if err != nil {
		logs.Error("访问Google Cloud Vision API失败")
		session.SendText("无法获取表情", session.Bot.UserName, to)
		return
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	web, err := sj.NewJson(body)
	if err != nil {
		logs.Error("解析Google Cloud Vision API响应错误")
		session.SendText("无法获取表情", session.Bot.UserName, to)
		return
	}
	if err, ok := web.CheckGet("error"); ok {
		session.SendText(err.Get("message").MustString("Unexpected Google Cloud Vision API response"), session.Bot.UserName, to)
		return
	}
	arr := web.Get("responses").GetIndex(0).Get("webDetection").Get("visuallySimilarImages")

	var us []string
	for i := 0; i < len(arr.MustArray()); i++ {
		us = append(us, arr.GetIndex(i).Get("url").MustString())
	}
	rt, rc := g.resCB(us)
	if len(rc) == 0 {
		session.SendText("没有找到匹配的动图", session.Bot.UserName, to)
	} else {
		switch rt {
		case wxweb.MSG_EMOTION:
			for _, v := range rc {
				time.Sleep(time.Millisecond * 1000) // delay 1s for each
				session.SendEmotionFromBytes(v.([]byte), session.Bot.UserName, to)
			}
		case wxweb.MSG_TEXT:
			for _, v := range rc {
				session.SendText(v.(string), session.Bot.UserName, to)
			}
		}
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

func url2Bytes(us []string) (rt int, rc []interface{}) {
	rt = wxweb.MSG_EMOTION
	rc = []interface{}{}
	for _, url := range us {
		b := downloadImage(url)
		if len(b) > 0 {
			rc = append(rc, b)
		} else {
			// session.SendText("获取图片失败", session.Bot.UserName, to)
		}
	}
	return
}

func requestAlways(*wxweb.Session, *wxweb.ReceivedMessage) bool {
	return true
}

// ------------------------------------------------------------------------------

type Ggifer struct {
	apiKey string // Google Cloud Platform API Key
	resCB  func([]string) (int, []interface{})
	reqFlt func(*wxweb.Session, *wxweb.ReceivedMessage) bool
}

func (g *Ggifer) Register(session *wxweb.Session) {
	if g.apiKey == "" {
		logs.Error("Google Cloud Platform API Key should be set before Register()")
		return
	}
	session.HandlerRegister.Add(g, wxweb.MSG_EMOTION, wxweb.Handler(requestSimilarImages), "google-gifer")
	session.HandlerRegister.Add(g, wxweb.MSG_LINK, wxweb.Handler(requestSimilarImages), "google-gifer_link")
}

// New ...
func New(c map[string]interface{}) (*Ggifer, error) {
	var ok bool
	var g Ggifer
	if keyExist(c, "APIKey") {
		if g.apiKey, ok = c["APIKey"].(string); !ok {
			return nil, errors.New("Unexpected APIKey type")
		}
	} else {
		g.apiKey = ""
	}
	if keyExist(c, "ResCallback") {
		if g.resCB, ok = c["ResCallback"].(func([]string) (int, []interface{})); !ok {
			return nil, errors.New("Unexpected ResCallback type")
		}
	} else {
		g.resCB = url2Bytes
	}
	if keyExist(c, "ReqFilter") {
		if g.reqFlt, ok = c["ReqFilter"].(func(*wxweb.Session, *wxweb.ReceivedMessage) bool); !ok {
			return nil, errors.New("Unexpected ReqFilter type")
		}
	} else {
		g.reqFlt = requestAlways
	}
	return &g, nil
}

func keyExist(m map[string]interface{}, k string) bool {
	if _, ok := m[k]; ok {
		return true
	}
	return false
}
