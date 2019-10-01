//
// Copyright (c) 2019 Ted Unangst <tedu@tedunangst.com>
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that the above
// copyright notice and this permission notice appear in all copies.
//
// THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
// WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
// MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
// ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
// WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
// ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
// OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.

package main

import (
	"bytes"
	"crypto/rsa"
	"database/sql"
	"fmt"
	"io"
	"log"
	notrand "math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"humungus.tedunangst.com/r/webs/httpsig"
	"humungus.tedunangst.com/r/webs/image"
	"humungus.tedunangst.com/r/webs/junk"
)

var theonetruename = `application/ld+json; profile="https://www.w3.org/ns/activitystreams"`
var thefakename = `application/activity+json`
var falsenames = []string{
	`application/ld+json`,
	`application/activity+json`,
}
var itiswhatitis = "https://www.w3.org/ns/activitystreams"
var thewholeworld = "https://www.w3.org/ns/activitystreams#Public"

func friendorfoe(ct string) bool {
	ct = strings.ToLower(ct)
	for _, at := range falsenames {
		if strings.HasPrefix(ct, at) {
			return true
		}
	}
	return false
}

func PostJunk(keyname string, key *rsa.PrivateKey, url string, j junk.Junk) error {
	var buf bytes.Buffer
	j.Write(&buf)
	return PostMsg(keyname, key, url, buf.Bytes())
}

func PostMsg(keyname string, key *rsa.PrivateKey, url string, msg []byte) error {
	client := http.DefaultClient
	req, err := http.NewRequest("POST", url, bytes.NewReader(msg))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "honksnonk/5.0; "+serverName)
	req.Header.Set("Content-Type", theonetruename)
	httpsig.SignRequest(keyname, key, req, msg)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	switch resp.StatusCode {
	case 200:
	case 201:
	case 202:
	default:
		return fmt.Errorf("http post status: %d", resp.StatusCode)
	}
	log.Printf("successful post: %s %d", url, resp.StatusCode)
	return nil
}

func GetJunk(url string) (junk.Junk, error) {
	return GetJunkTimeout(url, 30*time.Second)
}

func GetJunkFast(url string) (junk.Junk, error) {
	return GetJunkTimeout(url, 5*time.Second)
}

func GetJunkHardMode(url string) (junk.Junk, error) {
	j, err := GetJunk(url)
	if err != nil {
		emsg := err.Error()
		if emsg == "http get status: 502" || strings.Contains(emsg, "timeout") {
			log.Printf("trying again after error: %s", emsg)
			time.Sleep(time.Duration(60+notrand.Int63n(60)) * time.Second)
			j, err = GetJunk(url)
			if err != nil {
				log.Printf("still couldn't get it")
			} else {
				log.Printf("retry success!")
			}
		}
	}
	return j, err
}

func GetJunkTimeout(url string, timeout time.Duration) (junk.Junk, error) {
	at := thefakename
	if strings.Contains(url, ".well-known/webfinger?resource") {
		at = "application/jrd+json"
	}
	return junk.Get(url, junk.GetArgs{
		Accept:  at,
		Agent:   "honksnonk/5.0; " + serverName,
		Timeout: timeout,
	})
}

func savedonk(url string, name, desc, media string, localize bool) *Donk {
	if url == "" {
		return nil
	}
	var donk Donk
	row := stmtFindFile.QueryRow(url)
	err := row.Scan(&donk.FileID)
	if err == nil {
		return &donk
	}
	log.Printf("saving donk: %s", url)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("error querying: %s", err)
	}
	xid := xfiltrate()
	data := []byte{}
	if localize {
		resp, err := http.Get(url)
		if err != nil {
			log.Printf("error fetching %s: %s", url, err)
			localize = false
			goto saveit
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			localize = false
			goto saveit
		}
		var buf bytes.Buffer
		io.Copy(&buf, resp.Body)

		data = buf.Bytes()
		if strings.HasPrefix(media, "image") {
			img, err := image.Vacuum(&buf,
				image.Params{LimitSize: 4800 * 4800, MaxWidth: 2048, MaxHeight: 2048})
			if err != nil {
				log.Printf("unable to decode image: %s", err)
				localize = false
				data = []byte{}
				goto saveit
			}
			data = img.Data
			format := img.Format
			media = "image/" + format
			if format == "jpeg" {
				format = "jpg"
			}
			xid = xid + "." + format
		} else if len(data) > 100000 {
			log.Printf("not saving large attachment")
			localize = false
			data = []byte{}
		}
	}
saveit:
	fileid, err := savefile(xid, name, desc, url, media, localize, data)
	if err != nil {
		log.Printf("error saving file %s: %s", url, err)
		return nil
	}
	donk.FileID = fileid
	return &donk
}

func iszonked(userid int64, xid string) bool {
	row := stmtFindZonk.QueryRow(userid, xid)
	var id int64
	err := row.Scan(&id)
	if err == nil {
		return true
	}
	if err != sql.ErrNoRows {
		log.Printf("err querying zonk: %s", err)
	}
	return false
}

func needxonk(user *WhatAbout, x *Honk) bool {
	if thoudostbitethythumb(user.ID, x.Audience, x.XID) {
		log.Printf("not saving thumb biter? %s via %s", x.XID, x.Honker)
		return false
	}
	return needxonkid(user, x.XID)
}
func needxonkid(user *WhatAbout, xid string) bool {
	if strings.HasPrefix(xid, user.URL+"/") {
		return false
	}
	if thoudostbitethythumb(user.ID, nil, xid) {
		log.Printf("don't need thumb biter? %s", xid)
		return false
	}
	if iszonked(user.ID, xid) {
		log.Printf("already zonked: %s", xid)
		return false
	}
	row := stmtFindXonk.QueryRow(user.ID, xid)
	var id int64
	err := row.Scan(&id)
	if err == nil {
		return false
	}
	if err != sql.ErrNoRows {
		log.Printf("err querying xonk: %s", err)
	}
	return true
}

func eradicatexonk(userid int64, xid string) {
	xonk := getxonk(userid, xid)
	if xonk != nil {
		deletehonk(xonk.ID)
	}
	_, err := stmtSaveZonker.Exec(userid, xid, "zonk")
	if err != nil {
		log.Printf("error eradicating: %s", err)
	}
}

func savexonk(x *Honk) {
	log.Printf("saving xonk: %s", x.XID)
	go prehandle(x.Honker)
	go prehandle(x.Oonker)
	savehonk(x)
}

type Box struct {
	In     string
	Out    string
	Shared string
}

var boxofboxes = make(map[string]*Box)
var boxlock sync.Mutex
var boxinglock sync.Mutex

func getboxes(ident string) (*Box, error) {
	boxlock.Lock()
	b, ok := boxofboxes[ident]
	boxlock.Unlock()
	if ok {
		return b, nil
	}

	boxinglock.Lock()
	defer boxinglock.Unlock()

	boxlock.Lock()
	b, ok = boxofboxes[ident]
	boxlock.Unlock()
	if ok {
		return b, nil
	}

	var info string
	row := stmtGetXonker.QueryRow(ident, "boxes")
	err := row.Scan(&info)
	if err != nil {
		j, err := GetJunk(ident)
		if err != nil {
			return nil, err
		}
		inbox, _ := j.GetString("inbox")
		outbox, _ := j.GetString("outbox")
		sbox, _ := j.FindString([]string{"endpoints", "sharedInbox"})
		b = &Box{In: inbox, Out: outbox, Shared: sbox}
		if inbox != "" {
			m := strings.Join([]string{inbox, outbox, sbox}, " ")
			_, err = stmtSaveXonker.Exec(ident, m, "boxes")
			if err != nil {
				log.Printf("error saving boxes: %s", err)
			}
		}
	} else {
		m := strings.Split(info, " ")
		b = &Box{In: m[0], Out: m[1], Shared: m[2]}
	}

	boxlock.Lock()
	boxofboxes[ident] = b
	boxlock.Unlock()
	return b, nil
}

func gimmexonks(user *WhatAbout, outbox string) {
	log.Printf("getting outbox: %s", outbox)
	j, err := GetJunk(outbox)
	if err != nil {
		log.Printf("error getting outbox: %s", err)
		return
	}
	t, _ := j.GetString("type")
	origin := originate(outbox)
	if t == "OrderedCollection" {
		items, _ := j.GetArray("orderedItems")
		if items == nil {
			obj, ok := j.GetMap("first")
			if ok {
				items, _ = obj.GetArray("orderedItems")
			} else {
				page1, _ := j.GetString("first")
				j, err = GetJunk(page1)
				if err != nil {
					log.Printf("error gettings page1: %s", err)
					return
				}
				items, _ = j.GetArray("orderedItems")
			}
		}
		if len(items) > 20 {
			items = items[0:20]
		}
		for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
			items[i], items[j] = items[j], items[i]
		}
		for _, item := range items {
			obj, ok := item.(junk.Junk)
			if !ok {
				continue
			}
			xonk := xonkxonk(user, obj, origin)
			if xonk != nil {
				savexonk(xonk)
			}
		}
	}
}

func peeppeep() {
	user, _ := butwhatabout("htest")
	honkers := gethonkers(user.ID)
	for _, f := range honkers {
		if f.Flavor != "peep" {
			continue
		}
		log.Printf("getting updates: %s", f.XID)
		box, err := getboxes(f.XID)
		if err != nil {
			log.Printf("error getting outbox: %s", err)
			continue
		}
		gimmexonks(user, box.Out)
	}
}
func whosthere(xid string) ([]string, string) {
	obj, err := GetJunk(xid)
	if err != nil {
		log.Printf("error getting remote xonk: %s", err)
		return nil, ""
	}
	convoy, _ := obj.GetString("context")
	if convoy == "" {
		convoy, _ = obj.GetString("conversation")
	}
	return newphone(nil, obj), convoy
}

func newphone(a []string, obj junk.Junk) []string {
	for _, addr := range []string{"to", "cc", "attributedTo"} {
		who, _ := obj.GetString(addr)
		if who != "" {
			a = append(a, who)
		}
		whos, _ := obj.GetArray(addr)
		for _, w := range whos {
			who, _ := w.(string)
			if who != "" {
				a = append(a, who)
			}
		}
	}
	return a
}

func extractattrto(obj junk.Junk) string {
	who, _ := obj.GetString("attributedTo")
	if who != "" {
		return who
	}
	o, ok := obj.GetMap("attributedTo")
	if ok {
		id, ok := o.GetString("id")
		if ok {
			return id
		}
	}
	arr, _ := obj.GetArray("attributedTo")
	for _, a := range arr {
		o, ok := a.(junk.Junk)
		if ok {
			t, _ := o.GetString("type")
			id, _ := o.GetString("id")
			if t == "Person" || t == "" {
				return id
			}
		}
	}
	return ""
}

func consumeactivity(user *WhatAbout, j junk.Junk, origin string) {
	xonk := xonkxonk(user, j, origin)
	if xonk != nil {
		savexonk(xonk)
	}
}

func xonkxonk(user *WhatAbout, item junk.Junk, origin string) *Honk {
	depth := 0
	maxdepth := 10
	currenttid := ""
	var xonkxonkfn func(item junk.Junk, origin string) *Honk

	saveoneup := func(xid string) {
		log.Printf("getting oneup: %s", xid)
		if depth >= maxdepth {
			log.Printf("in too deep")
			return
		}
		obj, err := GetJunkHardMode(xid)
		if err != nil {
			log.Printf("error getting oneup: %s: %s", xid, err)
			return
		}
		depth++
		xonk := xonkxonkfn(obj, originate(xid))
		if xonk != nil {
			savexonk(xonk)
		}
		depth--
	}

	xonkxonkfn = func(item junk.Junk, origin string) *Honk {
		// id, _ := item.GetString( "id")
		what, _ := item.GetString("type")
		dt, _ := item.GetString("published")

		var err error
		var xid, rid, url, content, precis, convoy string
		var obj junk.Junk
		var ok bool
		isUpdate := false
		switch what {
		case "Delete":
			obj, ok = item.GetMap("object")
			if ok {
				xid, _ = obj.GetString("id")
			} else {
				xid, _ = item.GetString("object")
			}
			if xid == "" {
				return nil
			}
			if originate(xid) != origin {
				log.Printf("forged delete: %s", xid)
				return nil
			}
			log.Printf("eradicating %s", xid)
			eradicatexonk(user.ID, xid)
			return nil
		case "Tombstone":
			xid, _ = item.GetString("id")
			if xid == "" {
				return nil
			}
			if originate(xid) != origin {
				log.Printf("forged delete: %s", xid)
				return nil
			}
			log.Printf("eradicating %s", xid)
			eradicatexonk(user.ID, xid)
			return nil
		case "Announce":
			obj, ok = item.GetMap("object")
			if ok {
				xid, _ = obj.GetString("id")
			} else {
				xid, _ = item.GetString("object")
			}
			if !needxonkid(user, xid) {
				return nil
			}
			log.Printf("getting bonk: %s", xid)
			obj, err = GetJunkHardMode(xid)
			if err != nil {
				log.Printf("error getting bonk: %s: %s", xid, err)
			}
			origin = originate(xid)
			what = "bonk"
		case "Update":
			isUpdate = true
			fallthrough
		case "Create":
			obj, ok = item.GetMap("object")
			if !ok {
				xid, _ = item.GetString("object")
				log.Printf("getting created honk: %s", xid)
				obj, err = GetJunkHardMode(xid)
				if err != nil {
					log.Printf("error getting creation: %s", err)
				}
			}
			what = "honk"
		case "Read":
			xid, ok = item.GetString("object")
			if ok {
				if !needxonkid(user, xid) {
					log.Printf("don't need read obj: %s", xid)
					return nil
				}
				obj, err = GetJunkHardMode(xid)
				if err != nil {
					log.Printf("error getting read: %s", err)
					return nil
				}
				return xonkxonkfn(obj, originate(xid))
			}
			return nil
		case "Video":
			fallthrough
		case "Question":
			fallthrough
		case "Note":
			fallthrough
		case "Article":
			fallthrough
		case "Page":
			obj = item
			what = "honk"
		default:
			log.Printf("unknown activity: %s", what)
			fd, _ := os.OpenFile("savedinbox.json", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
			item.Write(fd)
			io.WriteString(fd, "\n")
			fd.Close()
			return nil
		}

		var xonk Honk
		// early init
		xonk.UserID = user.ID
		xonk.Honker, _ = item.GetString("actor")
		if obj != nil {
			if xonk.Honker == "" {
				xonk.Honker = extractattrto(obj)
			}
			xonk.Oonker = extractattrto(obj)
			if xonk.Oonker == xonk.Honker {
				xonk.Oonker = ""
			}
			xonk.Audience = newphone(nil, obj)
		}
		xonk.Audience = append(xonk.Audience, xonk.Honker)
		xonk.Audience = oneofakind(xonk.Audience)
		for _, a := range xonk.Audience {
			if a == user.URL {
				xonk.Whofore = 1
			}
		}

		if obj != nil {
			ot, _ := obj.GetString("type")
			url, _ = obj.GetString("url")
			dt2, ok := obj.GetString("published")
			if ok {
				dt = dt2
			}
			xid, _ = obj.GetString("id")
			precis, _ = obj.GetString("summary")
			if precis == "" {
				precis, _ = obj.GetString("name")
			}
			content, _ = obj.GetString("content")
			if !strings.HasPrefix(content, "<p>") {
				content = "<p>" + content
			}
			sens, _ := obj["sensitive"].(bool)
			if sens && precis == "" {
				precis = "unspecified horror"
			}
			rid, ok = obj.GetString("inReplyTo")
			if !ok {
				robj, ok := obj.GetMap("inReplyTo")
				if ok {
					rid, _ = robj.GetString("id")
				}
			}
			convoy, _ = obj.GetString("context")
			if strings.HasSuffix(convoy, "#context") &&
				originate(convoy) != originate(xid) {
				// friendica...
				convoy = ""
			}
			if convoy == "" {
				convoy, _ = obj.GetString("conversation")
			}
			if ot == "Question" {
				if what == "honk" {
					what = "qonk"
				}
				content += "<ul>"
				ans, _ := obj.GetArray("oneOf")
				for _, ai := range ans {
					a, ok := ai.(junk.Junk)
					if !ok {
						continue
					}
					as, _ := a.GetString("name")
					content += "<li>" + as
				}
				ans, _ = obj.GetArray("anyOf")
				for _, ai := range ans {
					a, ok := ai.(junk.Junk)
					if !ok {
						continue
					}
					as, _ := a.GetString("name")
					content += "<li>" + as
				}
				content += "</ul>"
			}
			if what == "honk" && rid != "" {
				what = "tonk"
			}
			atts, _ := obj.GetArray("attachment")
			for i, atti := range atts {
				att, ok := atti.(junk.Junk)
				if !ok {
					continue
				}
				at, _ := att.GetString("type")
				mt, _ := att.GetString("mediaType")
				u, _ := att.GetString("url")
				name, _ := att.GetString("name")
				desc, _ := att.GetString("summary")
				if desc == "" {
					desc = name
				}
				localize := false
				if i > 4 {
					log.Printf("excessive attachment: %s", at)
				} else if at == "Document" || at == "Image" {
					mt = strings.ToLower(mt)
					log.Printf("attachment: %s %s", mt, u)
					if mt == "text/plain" || strings.HasPrefix(mt, "image") {
						localize = true
					}
				} else {
					log.Printf("unknown attachment: %s", at)
				}
				if skipMedia(&xonk) {
					localize = false
				}
				donk := savedonk(u, name, desc, mt, localize)
				if donk != nil {
					xonk.Donks = append(xonk.Donks, donk)
				}
			}
			tags, _ := obj.GetArray("tag")
			for _, tagi := range tags {
				tag, ok := tagi.(junk.Junk)
				if !ok {
					continue
				}
				tt, _ := tag.GetString("type")
				name, _ := tag.GetString("name")
				desc, _ := tag.GetString("summary")
				if desc == "" {
					desc = name
				}
				if tt == "Emoji" {
					icon, _ := tag.GetMap("icon")
					mt, _ := icon.GetString("mediaType")
					if mt == "" {
						mt = "image/png"
					}
					u, _ := icon.GetString("url")
					donk := savedonk(u, name, desc, mt, true)
					if donk != nil {
						xonk.Donks = append(xonk.Donks, donk)
					}
				}
				if tt == "Hashtag" {
					if len(name) > 1 && name[0] == '#' {
						xonk.Onts = append(xonk.Onts, name)
					} else {
						log.Printf("bogus hashtag name: %s", name)
					}
				}
				if tt == "Place" {
					p := new(Place)
					p.Name = name
					p.Latitude, _ = tag["latitude"].(float64)
					p.Longitude, _ = tag["longitude"].(float64)
					p.Url, _ = tag.GetString("url")
					xonk.Place = p
				}
			}
			xonk.Onts = oneofakind(xonk.Onts)
		}
		if originate(xid) != origin {
			log.Printf("original sin: %s <> %s", xid, origin)
			item.Write(os.Stdout)
			return nil
		}

		if currenttid == "" {
			currenttid = convoy
		}

		// init xonk
		xonk.What = what
		xonk.XID = xid
		xonk.RID = rid
		xonk.Date, _ = time.Parse(time.RFC3339, dt)
		xonk.URL = url
		xonk.Noise = content
		xonk.Precis = precis
		xonk.Format = "html"

		if isUpdate {
			log.Printf("something has changed! %s", xonk.XID)
			prev := getxonk(user.ID, xonk.XID)
			if prev == nil {
				log.Printf("didn't find old version for update: %s", xonk.XID)
				return nil
			}
			prev.Noise = xonk.Noise
			prev.Precis = xonk.Precis
			prev.Date = xonk.Date
			prev.Donks = xonk.Donks
			prev.Onts = xonk.Onts
			prev.Place = xonk.Place
			updatehonk(prev)
			return nil
		}

		if needxonk(user, &xonk) {
			if rid != "" {
				if needxonkid(user, rid) {
					saveoneup(rid)
				}
				if convoy == "" {
					xx := getxonk(user.ID, rid)
					if xx != nil {
						convoy = xx.Convoy
					}
				}
			}
			if convoy == "" {
				convoy = currenttid
			}
			xonk.Convoy = convoy
			return &xonk
		}
		return nil
	}

	return xonkxonkfn(item, origin)
}

func rubadubdub(user *WhatAbout, req junk.Junk) {
	xid, _ := req.GetString("id")
	actor, _ := req.GetString("actor")
	j := junk.New()
	j["@context"] = itiswhatitis
	j["id"] = user.URL + "/dub/" + url.QueryEscape(xid)
	j["type"] = "Accept"
	j["actor"] = user.URL
	j["to"] = actor
	j["published"] = time.Now().UTC().Format(time.RFC3339)
	j["object"] = req

	var buf bytes.Buffer
	j.Write(&buf)
	msg := buf.Bytes()

	deliverate(0, user.Name, actor, msg)
}

func itakeitallback(user *WhatAbout, xid string) {
	j := junk.New()
	j["@context"] = itiswhatitis
	j["id"] = user.URL + "/unsub/" + url.QueryEscape(xid)
	j["type"] = "Undo"
	j["actor"] = user.URL
	j["to"] = xid
	f := junk.New()
	f["id"] = user.URL + "/sub/" + url.QueryEscape(xid)
	f["type"] = "Follow"
	f["actor"] = user.URL
	f["to"] = xid
	f["object"] = xid
	j["object"] = f
	j["published"] = time.Now().UTC().Format(time.RFC3339)

	var buf bytes.Buffer
	j.Write(&buf)
	msg := buf.Bytes()

	deliverate(0, user.Name, xid, msg)
}

func subsub(user *WhatAbout, xid string) {
	j := junk.New()
	j["@context"] = itiswhatitis
	j["id"] = user.URL + "/sub/" + url.QueryEscape(xid)
	j["type"] = "Follow"
	j["actor"] = user.URL
	j["to"] = xid
	j["object"] = xid
	j["published"] = time.Now().UTC().Format(time.RFC3339)

	var buf bytes.Buffer
	j.Write(&buf)
	msg := buf.Bytes()

	deliverate(0, user.Name, xid, msg)
}

// returns activity, object
func jonkjonk(user *WhatAbout, h *Honk) (junk.Junk, junk.Junk) {
	dt := h.Date.Format(time.RFC3339)
	var jo junk.Junk
	j := junk.New()
	j["id"] = user.URL + "/" + h.What + "/" + shortxid(h.XID)
	j["actor"] = user.URL
	j["published"] = dt
	if h.Public {
		j["to"] = []string{h.Audience[0], user.URL + "/followers"}
	} else {
		j["to"] = h.Audience[0]
	}
	if len(h.Audience) > 1 {
		j["cc"] = h.Audience[1:]
	}

	switch h.What {
	case "update":
		fallthrough
	case "tonk":
		fallthrough
	case "honk":
		if h.What == "update" {
			j["type"] = "Update"
		} else {
			j["type"] = "Create"
		}

		jo = junk.New()
		jo["id"] = h.XID
		jo["type"] = "Note"
		jo["published"] = dt
		jo["url"] = h.XID
		jo["attributedTo"] = user.URL
		if h.RID != "" {
			jo["inReplyTo"] = h.RID
		}
		if h.Convoy != "" {
			jo["context"] = h.Convoy
			jo["conversation"] = h.Convoy
		}
		jo["to"] = h.Audience[0]
		if len(h.Audience) > 1 {
			jo["cc"] = h.Audience[1:]
		}
		if !h.Public {
			jo["directMessage"] = true
		}
		translate(h)
		h.Noise = re_memes.ReplaceAllString(h.Noise, "")
		jo["summary"] = h.Precis
		jo["content"] = ontologize(mentionize(h.Noise))
		if strings.HasPrefix(h.Precis, "DZ:") {
			jo["sensitive"] = true
		}

		var replies []string
		for _, reply := range h.Replies {
			replies = append(replies, reply.XID)
		}
		if len(replies) > 0 {
			jr := junk.New()
			jr["type"] = "Collection"
			jr["totalItems"] = len(replies)
			jr["items"] = replies
			jo["replies"] = jr
		}

		var tags []junk.Junk
		g := bunchofgrapes(h.Noise)
		for _, m := range g {
			t := junk.New()
			t["type"] = "Mention"
			t["name"] = m.who
			t["href"] = m.where
			tags = append(tags, t)
		}
		for _, o := range h.Onts {
			t := junk.New()
			t["type"] = "Hashtag"
			o = strings.ToLower(o)
			t["href"] = fmt.Sprintf("https://%s/o/%s", serverName, o[1:])
			t["name"] = o
			tags = append(tags, t)
		}
		herd := herdofemus(h.Noise)
		for _, e := range herd {
			t := junk.New()
			t["id"] = e.ID
			t["type"] = "Emoji"
			t["name"] = e.Name
			i := junk.New()
			i["type"] = "Image"
			i["mediaType"] = "image/png"
			i["url"] = e.ID
			t["icon"] = i
			tags = append(tags, t)
		}
		if p := h.Place; p != nil {
			t := junk.New()
			t["type"] = "Place"
			t["name"] = p.Name
			t["latitude"] = p.Latitude
			t["longitude"] = p.Longitude
			t["url"] = p.Url
			tags = append(tags, t)
		}
		if len(tags) > 0 {
			jo["tag"] = tags
		}
		var atts []junk.Junk
		for _, d := range h.Donks {
			if re_emus.MatchString(d.Name) {
				continue
			}
			jd := junk.New()
			jd["mediaType"] = d.Media
			jd["name"] = d.Name
			jd["summary"] = d.Desc
			jd["type"] = "Document"
			jd["url"] = d.URL
			atts = append(atts, jd)
		}
		if len(atts) > 0 {
			jo["attachment"] = atts
		}
		j["object"] = jo
	case "bonk":
		j["type"] = "Announce"
		if h.Convoy != "" {
			j["context"] = h.Convoy
		}
		j["object"] = h.XID
	case "unbonk":
		b := junk.New()
		b["id"] = user.URL + "/" + "bonk" + "/" + shortxid(h.XID)
		b["type"] = "Announce"
		b["actor"] = user.URL
		if h.Convoy != "" {
			b["context"] = h.Convoy
		}
		b["object"] = h.XID
		j["type"] = "Undo"
		j["object"] = b
	case "zonk":
		j["type"] = "Delete"
		j["object"] = h.XID
	case "ack":
		j["type"] = "Read"
		j["object"] = h.XID
	case "deack":
		b := junk.New()
		b["id"] = user.URL + "/" + "ack" + "/" + shortxid(h.XID)
		b["type"] = "Read"
		b["actor"] = user.URL
		b["object"] = h.XID
		j["type"] = "Undo"
		j["object"] = b
	}

	return j, jo
}

func honkworldwide(user *WhatAbout, honk *Honk) {
	jonk, _ := jonkjonk(user, honk)
	jonk["@context"] = itiswhatitis
	var buf bytes.Buffer
	jonk.Write(&buf)
	msg := buf.Bytes()

	rcpts := make(map[string]bool)
	for _, a := range honk.Audience {
		if a == thewholeworld || a == user.URL || strings.HasSuffix(a, "/followers") {
			continue
		}
		box, _ := getboxes(a)
		if box != nil && honk.Public && box.Shared != "" {
			rcpts["%"+box.Shared] = true
		} else {
			rcpts[a] = true
		}
	}
	if honk.Public {
		for _, f := range getdubs(user.ID) {
			if f.XID == user.URL {
				continue
			}
			box, _ := getboxes(f.XID)
			if box != nil && box.Shared != "" {
				rcpts["%"+box.Shared] = true
			} else {
				rcpts[f.XID] = true
			}
		}
	}
	for a := range rcpts {
		go deliverate(0, user.Name, a, msg)
	}
}

func asjonker(user *WhatAbout) junk.Junk {
	about := obfusbreak(user.About)

	j := junk.New()
	j["@context"] = itiswhatitis
	j["id"] = user.URL
	j["type"] = "Person"
	j["inbox"] = user.URL + "/inbox"
	j["outbox"] = user.URL + "/outbox"
	j["followers"] = user.URL + "/followers"
	j["following"] = user.URL + "/following"
	j["name"] = user.Display
	j["preferredUsername"] = user.Name
	j["summary"] = about
	j["url"] = user.URL
	a := junk.New()
	a["type"] = "Image"
	a["mediaType"] = "image/png"
	a["url"] = fmt.Sprintf("https://%s/a?a=%s", serverName, url.QueryEscape(user.URL))
	j["icon"] = a
	k := junk.New()
	k["id"] = user.URL + "#key"
	k["owner"] = user.URL
	k["publicKeyPem"] = user.Key
	j["publicKey"] = k

	return j
}

var handfull = make(map[string]string)
var handlock sync.Mutex

func gofish(name string) string {
	if name[0] == '@' {
		name = name[1:]
	}
	m := strings.Split(name, "@")
	if len(m) != 2 {
		log.Printf("bad fish name: %s", name)
		return ""
	}
	handlock.Lock()
	ref, ok := handfull[name]
	handlock.Unlock()
	if ok {
		return ref
	}
	row := stmtGetXonker.QueryRow(name, "fishname")
	var href string
	err := row.Scan(&href)
	if err == nil {
		handlock.Lock()
		handfull[name] = href
		handlock.Unlock()
		return href
	}
	log.Printf("fishing for %s", name)
	j, err := GetJunkFast(fmt.Sprintf("https://%s/.well-known/webfinger?resource=acct:%s", m[1], name))
	if err != nil {
		log.Printf("failed to go fish %s: %s", name, err)
		handlock.Lock()
		handfull[name] = ""
		handlock.Unlock()
		return ""
	}
	links, _ := j.GetArray("links")
	for _, li := range links {
		l, ok := li.(junk.Junk)
		if !ok {
			continue
		}
		href, _ := l.GetString("href")
		rel, _ := l.GetString("rel")
		t, _ := l.GetString("type")
		if rel == "self" && friendorfoe(t) {
			_, err := stmtSaveXonker.Exec(name, href, "fishname")
			if err != nil {
				log.Printf("error saving fishname: %s", err)
			}
			handlock.Lock()
			handfull[name] = href
			handlock.Unlock()
			return href
		}
	}
	handlock.Lock()
	handfull[name] = ""
	handlock.Unlock()
	return ""
}

func isactor(t string) bool {
	switch t {
	case "Person":
	case "Organization":
	case "Application":
	case "Service":
	default:
		return false
	}
	return true
}

func investigate(name string) (*Honker, error) {
	if name == "" {
		return nil, fmt.Errorf("no name")
	}
	if name[0] == '@' {
		name = gofish(name)
	}
	if name == "" {
		return nil, fmt.Errorf("no name")
	}
	log.Printf("digging up some info on %s", name)
	obj, err := GetJunkFast(name)
	if err != nil {
		log.Printf("error investigating honker: %s", err)
		return nil, err
	}
	t, _ := obj.GetString("type")
	if !isactor(t) {
		log.Printf("it's not a person! %s", name)
		return nil, err
	}
	xid, _ := obj.GetString("id")
	handle, _ := obj.GetString("preferredUsername")
	return &Honker{XID: xid, Handle: handle}, nil
}
