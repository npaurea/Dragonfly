/*
 * Copyright 1999-2018 Alibaba Group.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package config

import (
	"bytes"
	"fmt"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/alibaba/Dragonfly/dfget/util"
	"github.com/go-check/check"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

type ConfigSuite struct{}

func init() {
	check.Suite(&ConfigSuite{})
}

func (suite *ConfigSuite) SetUpTest(c *check.C) {
	Reset()
}

func (suite *ConfigSuite) TestContext_String(c *check.C) {
	expected := "{\"url\":\"\",\"output\":\"\""
	c.Assert(strings.Contains(Ctx.String(), expected), check.Equals, true)
	Ctx.LocalLimit = 20971520
	Ctx.Pattern = "p2p"
	Ctx.Version = true
	expected = "\"url\":\"\",\"output\":\"\",\"localLimit\":20971520," +
		"\"pattern\":\"p2p\",\"version\":true"
	c.Assert(strings.Contains(Ctx.String(), expected), check.Equals, true)
}

func (suite *ConfigSuite) TestNewContext(c *check.C) {
	before := time.Now()
	time.Sleep(time.Millisecond)
	Ctx = NewContext()
	time.Sleep(time.Millisecond)
	after := time.Now()

	c.Assert(Ctx.StartTime.After(before), check.Equals, true)
	c.Assert(Ctx.StartTime.Before(after), check.Equals, true)

	beforeSign := fmt.Sprintf("%d-%.3f",
		os.Getpid(), float64(before.UnixNano())/float64(time.Second))
	afterSign := fmt.Sprintf("%d-%.3f",
		os.Getpid(), float64(after.UnixNano())/float64(time.Second))
	c.Assert(beforeSign < Ctx.Sign, check.Equals, true)
	c.Assert(afterSign > Ctx.Sign, check.Equals, true)

	if curUser, err := user.Current(); err != nil {
		c.Assert(Ctx.User, check.Equals, curUser.Username)
		c.Assert(Ctx.WorkHome, check.Equals, path.Join(curUser.HomeDir, ".small-dragonfly"))
	}
}

func (suite *ConfigSuite) TestAssertContext(c *check.C) {
	var (
		clog = logrus.StandardLogger()
		buf  = &bytes.Buffer{}
	)
	clog.Out = buf

	var cases = []struct {
		clog     *logrus.Logger
		slog     *logrus.Logger
		url      string
		output   string
		expected string
	}{
		{expected: "client log"},
		{clog: clog, expected: "server log"},
		{clog: clog, slog: clog, expected: "invalid url"},
		{clog: clog, slog: clog, url: "http://a.b", expected: ""},
		{clog: clog, slog: clog, url: "http://a.b", output: "/root", expected: "invalid output"},
	}

	var f = func() (msg string) {
		defer func() {
			if r := recover(); r != nil {
				switch r := r.(type) {
				case error:
					msg = r.Error()
				case *logrus.Entry:
					msg = r.Message
					buf.Reset()
				default:
					msg = fmt.Sprintf("%v", r)
				}
			}
		}()
		AssertContext(Ctx)
		return ""
	}

	for _, v := range cases {
		Ctx.ClientLogger = v.clog
		Ctx.ServerLogger = v.slog
		Ctx.URL = v.url
		Ctx.Output = v.output
		actual := f()
		c.Assert(strings.HasPrefix(actual, v.expected), check.Equals, true,
			check.Commentf("actual:[%s] expected:[%s]", actual, v.expected))
	}
}

func (suite *ConfigSuite) TestCheckURL(c *check.C) {
	var cases = map[string]bool{
		"":                     false,
		"abcdefg":              false,
		"////a//":              false,
		"a////a//":             false,
		"a.com////a//":         true,
		"127.0.0.1":            true,
		"127.0.0.1:":           true,
		"127.0.0.1:8080":       true,
		"127.0.0.1:8080/我":     true,
		"127.0.0.1:8080/我?x=1": true,
		"a.b":            true,
		"www.taobao.com": true,
		"https://github.com/alibaba/Dragonfly/issues?" +
			"q=is%3Aissue+is%3Aclosed": true,
	}

	c.Assert(checkURL(Ctx), check.NotNil)
	for k, v := range cases {
		for _, scheme := range []string{"http", "https", "HTTP", "HTTPS"} {
			Ctx.URL = fmt.Sprintf("%s://%s", scheme, k)
			actual := fmt.Sprintf("%s:%v", k, checkURL(Ctx))
			expected := fmt.Sprintf("%s:%s://%s", k, scheme, k)
			if v {
				expected = fmt.Sprintf("%s:<nil>", k)
			}
			c.Assert(actual, check.Equals, expected)
		}
	}
}

func (suite *ConfigSuite) TestCheckOutput(c *check.C) {
	curDir, _ := filepath.Abs(".")

	var j = func(p string) string { return filepath.Join(curDir, p) }
	var cases = []struct {
		url      string
		output   string
		expected string
	}{
		{"http://www.taobao.com", "", j("www.taobao.com")},
		{"http://www.taobao.com", "/tmp/zj.test", "/tmp/zj.test"},
		{"www.taobao.com", "", ""},
		{"www.taobao.com", "/tmp/zj.test", "/tmp/zj.test"},
		{"", "/tmp/zj.test", "/tmp/zj.test"},
		{"", "zj.test", j("zj.test")},
		{"", "/tmp", ""},
		{"", "/tmp/a/b/c/d/e/zj.test", "/tmp/a/b/c/d/e/zj.test"},
	}

	if Ctx.User != "root" {
		cases = append(cases, struct {
			url      string
			output   string
			expected string
		}{url: "", output: "/root/zj.test", expected: ""})
	}
	for _, v := range cases {
		Ctx.URL = v.url
		Ctx.Output = v.output
		if util.IsEmptyStr(v.expected) {
			c.Assert(checkOutput(Ctx), check.NotNil, check.Commentf("%v", v))
		} else {
			c.Assert(checkOutput(Ctx), check.IsNil, check.Commentf("%v", v))
			c.Assert(Ctx.Output, check.Equals, v.expected, check.Commentf("%v", v))
		}
	}
}
