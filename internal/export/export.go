// Package export renders a trip as files other tools understand: an iCalendar
// feed for the phone's calendar and a GPX file for maps and watches.
package export

import (
	"fmt"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-travel/internal/store"
)

func icsEscape(s string) string {
	r := strings.NewReplacer("\\", "\\\\", ";", "\\;", ",", "\\,", "\n", "\\n", "\r", "")
	return r.Replace(s)
}

func icsDate(d string) string { return strings.ReplaceAll(d[:10], "-", "") }

// icsLocal renders a wall-clock time (2026-08-28T07:30) as a floating local time.
func icsLocal(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 16 {
		return strings.ReplaceAll(s[:10], "-", "") + "T" + strings.ReplaceAll(s[11:16], ":", "") + "00"
	}
	if len(s) >= 10 {
		return icsDate(s)
	}
	return ""
}

// ICS renders a calendar with the trip itself, each stay, each leg and each scheduled place.
func ICS(t *store.Trip, places []store.Place, lodging []store.Lodging, transport []store.Transport, baseURL string) string {
	var b strings.Builder
	w := func(s string) { b.WriteString(s); b.WriteString("\r\n") }
	w("BEGIN:VCALENDAR")
	w("VERSION:2.0")
	w("PRODID:-//Pocket Travel//EN")
	w("CALSCALE:GREGORIAN")
	w("X-WR-CALNAME:" + icsEscape(t.Name))
	stamp := time.Now().UTC().Format("20060102T150405Z")
	uid := func(kind string, id int64) string {
		return fmt.Sprintf("pocket-travel-%s-%d-%d@pocket-travel", kind, t.ID, id)
	}
	link := ""
	if baseURL != "" {
		link = fmt.Sprintf("%s/trips/%d", baseURL, t.ID)
	}
	if t.StartDate != "" && t.EndDate != "" {
		end, _ := time.Parse("2006-01-02", t.EndDate)
		w("BEGIN:VEVENT")
		w("UID:" + uid("trip", t.ID))
		w("DTSTAMP:" + stamp)
		w("DTSTART;VALUE=DATE:" + icsDate(t.StartDate))
		w("DTEND;VALUE=DATE:" + end.AddDate(0, 0, 1).Format("20060102"))
		w("SUMMARY:" + icsEscape("✈ "+t.Name))
		if t.Destination != "" {
			w("LOCATION:" + icsEscape(t.Destination))
		}
		if link != "" {
			w("URL:" + link)
		}
		w("END:VEVENT")
	}
	for _, l := range lodging {
		if len(l.CheckIn) < 10 || len(l.CheckOut) < 10 {
			continue
		}
		end, err := time.Parse("2006-01-02", l.CheckOut[:10])
		if err != nil {
			continue
		}
		w("BEGIN:VEVENT")
		w("UID:" + uid("lodging", l.ID))
		w("DTSTAMP:" + stamp)
		w("DTSTART;VALUE=DATE:" + icsDate(l.CheckIn))
		w("DTEND;VALUE=DATE:" + end.Format("20060102"))
		w("SUMMARY:" + icsEscape("🛏 "+l.Name))
		if l.Address != "" {
			w("LOCATION:" + icsEscape(l.Address))
		}
		desc := []string{}
		if l.ReservationNumber != "" {
			desc = append(desc, "Reservation: "+l.ReservationNumber)
		}
		if l.Phone != "" {
			desc = append(desc, "Phone: "+l.Phone)
		}
		if l.Link != "" {
			desc = append(desc, l.Link)
		}
		if len(desc) > 0 {
			w("DESCRIPTION:" + icsEscape(strings.Join(desc, "\n")))
		}
		w("END:VEVENT")
	}
	for _, tr := range transport {
		start := icsLocal(tr.DepartAt)
		if start == "" {
			continue
		}
		end := icsLocal(tr.ArriveAt)
		w("BEGIN:VEVENT")
		w("UID:" + uid("transport", tr.ID))
		w("DTSTAMP:" + stamp)
		if len(start) == 8 {
			w("DTSTART;VALUE=DATE:" + start)
		} else {
			w("DTSTART:" + start)
			if end != "" && len(end) > 8 {
				w("DTEND:" + end)
			}
		}
		title := tr.Name
		if title == "" {
			title = tr.FromName + " → " + tr.ToName
		}
		w("SUMMARY:" + icsEscape(transportIcon(tr.Type)+" "+title))
		desc := []string{}
		if tr.Carrier != "" || tr.Number != "" {
			desc = append(desc, strings.TrimSpace(tr.Carrier+" "+tr.Number))
		}
		if tr.BookingRef != "" {
			desc = append(desc, "Booking: "+tr.BookingRef)
		}
		if tr.Seat != "" {
			desc = append(desc, "Seat: "+tr.Seat)
		}
		if len(desc) > 0 {
			w("DESCRIPTION:" + icsEscape(strings.Join(desc, "\n")))
		}
		if tr.FromName != "" {
			w("LOCATION:" + icsEscape(tr.FromName))
		}
		w("END:VEVENT")
	}
	for _, p := range places {
		if p.DayDate == "" {
			continue
		}
		w("BEGIN:VEVENT")
		w("UID:" + uid("place", p.ID))
		w("DTSTAMP:" + stamp)
		if p.StartTime != "" {
			w("DTSTART:" + icsLocal(p.DayDate+"T"+p.StartTime))
			dur := p.DurationMin
			if dur <= 0 {
				dur = 60
			}
			st, _ := time.Parse("2006-01-02T15:04", p.DayDate+"T"+p.StartTime)
			w("DTEND:" + st.Add(time.Duration(dur)*time.Minute).Format("20060102T150405"))
		} else {
			d, _ := time.Parse("2006-01-02", p.DayDate)
			w("DTSTART;VALUE=DATE:" + icsDate(p.DayDate))
			w("DTEND;VALUE=DATE:" + d.AddDate(0, 0, 1).Format("20060102"))
		}
		w("SUMMARY:" + icsEscape(p.Name))
		if p.Address != "" {
			w("LOCATION:" + icsEscape(p.Address))
		} else if p.Lat != nil && p.Lng != nil {
			w(fmt.Sprintf("GEO:%f;%f", *p.Lat, *p.Lng))
		}
		if p.Notes != "" || p.Link != "" {
			w("DESCRIPTION:" + icsEscape(strings.TrimSpace(p.Notes+"\n"+p.Link)))
		}
		w("END:VEVENT")
	}
	w("END:VCALENDAR")
	return b.String()
}

func transportIcon(t string) string {
	switch t {
	case "flight":
		return "✈"
	case "train":
		return "🚆"
	case "bus":
		return "🚌"
	case "car":
		return "🚗"
	case "ferry":
		return "⛴"
	case "taxi":
		return "🚕"
	case "bike":
		return "🚲"
	case "walk":
		return "🚶"
	}
	return "➡"
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;")
	return r.Replace(s)
}

// RoutePoint is a point of the recorded route.
type RoutePoint struct {
	Lat, Lng float64
	TS       int64
}

// GPX renders places and stays as waypoints and an optional recorded route as a track.
func GPX(t *store.Trip, places []store.Place, lodging []store.Lodging, route []RoutePoint) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<gpx version="1.1" creator="Pocket Travel" xmlns="http://www.topografix.com/GPX/1/1">` + "\n")
	fmt.Fprintf(&b, "  <metadata><name>%s</name></metadata>\n", xmlEscape(t.Name))
	for _, p := range places {
		if p.Lat == nil || p.Lng == nil {
			continue
		}
		fmt.Fprintf(&b, "  <wpt lat=\"%f\" lon=\"%f\"><name>%s</name><type>%s</type>", *p.Lat, *p.Lng, xmlEscape(p.Name), xmlEscape(p.Category))
		if p.Address != "" {
			fmt.Fprintf(&b, "<desc>%s</desc>", xmlEscape(p.Address))
		}
		if p.Link != "" {
			fmt.Fprintf(&b, "<link href=\"%s\"/>", xmlEscape(p.Link))
		}
		b.WriteString("</wpt>\n")
	}
	for _, l := range lodging {
		if l.Lat == nil || l.Lng == nil {
			continue
		}
		fmt.Fprintf(&b, "  <wpt lat=\"%f\" lon=\"%f\"><name>%s</name><type>lodging</type></wpt>\n", *l.Lat, *l.Lng, xmlEscape(l.Name))
	}
	if len(route) > 1 {
		fmt.Fprintf(&b, "  <trk><name>%s — recorded route</name><trkseg>\n", xmlEscape(t.Name))
		for _, p := range route {
			if p.TS > 0 {
				fmt.Fprintf(&b, "    <trkpt lat=\"%f\" lon=\"%f\"><time>%s</time></trkpt>\n", p.Lat, p.Lng, time.Unix(p.TS, 0).UTC().Format(time.RFC3339))
			} else {
				fmt.Fprintf(&b, "    <trkpt lat=\"%f\" lon=\"%f\"/>\n", p.Lat, p.Lng)
			}
		}
		b.WriteString("  </trkseg></trk>\n")
	}
	b.WriteString("</gpx>\n")
	return b.String()
}
