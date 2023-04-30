package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/arran4/golang-ical"
	"github.com/kelvins/sunrisesunset"
	"github.com/schollz/progressbar/v3"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	Latitude     = 32.842674
	Longitude    = -117.257767
	LocationName = "La Jolla, CA"
)
const (
	StartYear      = 2023
	StartMonth     = 2
	StartDay       = 10
	YearsToCompute = 1
	DaysToCompute  = YearsToCompute * 365
	// DaysToCompute = 10
)

const (
	// MaxThreads = 1
	MaxThreads = DaysToCompute / 2 // gives the best performance on my machine
	// MaxThreads = 80
)

type SunriseSunset struct {
	sr time.Time
	ss time.Time
}

type ByDate [][]SunriseSunset

func (a ByDate) Len() int           { return len(a) }
func (a ByDate) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByDate) Less(i, j int) bool { return a[i][0].sr.Compare(a[j][0].sr) == -1 }

func main() {
	PST, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		panic(err)
	}

	if MaxThreads > DaysToCompute {
		panic(fmt.Errorf("Max threads must be < #days"))
	}

	startDate := time.Date(StartYear, StartMonth, StartDay, 10, 0, 0, 0, time.UTC)
	p := sunrisesunset.Parameters{
		Latitude:  Latitude,
		Longitude: Longitude,
		UtcOffset: -8,
		Date:      startDate,
	}

	ch := make(chan []SunriseSunset)
	daysPerThread, extraDays := DaysToCompute/MaxThreads, DaysToCompute%MaxThreads

	start := startDate
	bar := progressbar.Default(DaysToCompute)
	for i := 0; i < MaxThreads; i++ {
		d := daysPerThread
		if extraDays > 0 {
			d++
			extraDays--
		}

		go func(ch chan []SunriseSunset, start time.Time, days int) {
			// fmt.Printf("computing %s\n", start)
			ch <- computeRange(p, PST, start, days, bar)
		}(ch, start, d)
		start = start.AddDate(0, 0, d)
	}

	collection := make([][]SunriseSunset, 0, MaxThreads)
	for i := 0; i < MaxThreads; i++ {
		collection = append(collection, <-ch)
	}

	sort.Sort(ByDate(collection))

	cal := ics.NewCalendar()
	cal.SetTzid("America/Los_Angeles")
	cal.SetXWRCalName("Suntimes for " + LocationName)

	prevSunrise, prevSunset := collection[0][0].sr, collection[0][0].ss
	sum := 0
	for _, group := range collection {
		for _, srss := range group {
			sunrise, sunset := srss.sr, srss.ss
			desc := genDescription(sunrise, prevSunrise, sunset, prevSunset)

			createEvent(cal, sunrise, desc, true)
			createEvent(cal, sunset, desc, false)

			prevSunrise, prevSunset = sunrise, sunset
			sum++
		}
	}
	s := cal.Serialize()

	// create output directory if it doesn't exist
	outputDir := filepath.Join(".", "output")
	err = os.MkdirAll(outputDir, os.ModePerm)
	if err != nil {
		panic(err)
	}
	f, err := os.OpenFile(filepath.Join(outputDir, "suntimes.ics"),
		os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	defer f.Close()
	if err != nil {
		log.Fatal(err)
	}
	f.WriteString(s)
	// fmt.Println(s)

	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
}

func genDescription(sr, psr, ss, pss time.Time) string {
	return fmt.Sprintf(`Sunset: %d:%02d PM
Change: %d s

Night length: %s

Sunrise: %d:%02d AM
Change: %d s`, ss.Hour()-12,
		ss.Minute(),
		int(pss.Sub(ss).Seconds()+86400),
		sr.Sub(pss).String(),
		sr.Hour(),
		sr.Minute(),
		int(psr.Sub(sr).Seconds()+86400),
	)
}

func computeRange(p sunrisesunset.Parameters, tz *time.Location,
	startDate time.Time, numDays int, bar *progressbar.ProgressBar) []SunriseSunset {
	ret := make([]SunriseSunset, 0, numDays)
	p.Date = startDate

	for i := 0; i < numDays; i++ {
		sunrise, sunset := sunriseSunsetTZ(&p, tz)
		ret = append(ret, SunriseSunset{sunrise, sunset})
		p.Date = p.Date.AddDate(0, 0, 1)
		bar.Add(1)
	}

	return ret
}

func sunriseSunsetTZ(p *sunrisesunset.Parameters, tz *time.Location) (time.Time, time.Time) {
	eightH, _ := time.ParseDuration("8h")
	sr, ss, err := p.GetSunriseSunset()

	if err != nil {
		panic(err)
	}
	// We add 8 hours to realign the time to UTC due to the offset of -8
	// passed into the Parameters struct
	// The initial offset is necessary for the sunset to be calculated correctly
	sr = sr.Add(eightH).In(tz)
	ss = ss.Add(eightH).In(tz)

	return sr, ss
}

func randomUID() string {
	p1, _ := randomHex(4)
	p2, _ := randomHex(2)
	p3, _ := randomHex(2)
	p4, _ := randomHex(2)
	p5, _ := randomHex(6)
	return strings.ToUpper(fmt.Sprintf("%s-%s-%s-%s-%s", p1, p2, p3, p4, p5))
}

func randomHex(n int) (string, error) {
	bytes := make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func createEvent(cal *ics.Calendar, event time.Time, desc string, isSunrise bool) {
	sunsetEvent := cal.AddEvent(randomUID())
	sunsetEvent.SetCreatedTime(time.Now())
	sunsetEvent.SetDtStampTime(time.Now())
	sunsetEvent.SetModifiedAt(time.Now())
	sunsetEvent.SetStartAt(event)
	sunsetEvent.SetEndAt(event)
	if isSunrise {
		sunsetEvent.SetSummary(fmt.Sprintf("☀️↑ %d:%02d AM Sunrise",
			event.Hour(), event.Minute()))
	} else {
		sunsetEvent.SetSummary(fmt.Sprintf("☀️↓ %d:%02d PM Sunset",
			event.Hour()-12, event.Minute()))
	}
	sunsetEvent.SetLocation(LocationName)
	sunsetEvent.SetDescription(desc)
}
