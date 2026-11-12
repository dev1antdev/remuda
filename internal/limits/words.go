package limits

import (
	"fmt"
	"math"
	"time"

	"github.com/dev1antdev/remuda/internal/push"
)

// AgentName is how a person calls the agent a window belongs to.
func AgentName(agent string) string {
	switch agent {
	case "claude":
		return "Claude"
	case "codex":
		return "Codex"
	}
	return agent
}

// WindowName is a window in words: "5-hour", "weekly".
func WindowName(name string) string {
	switch name {
	case "5h":
		return "5-hour"
	case "week":
		return "weekly"
	}
	return name
}

// Warning is the push for a window that reached 80 % or its limit.
func Warning(w Window, level int, machine string, now time.Time) push.Payload {
	title := fmt.Sprintf("%s's %s limit at %d %%", AgentName(w.Agent), WindowName(w.Name), int(math.Floor(w.Used)))
	body := "Resets " + ResetWords(w.ResetsAt, now)
	if level >= 100 {
		title = fmt.Sprintf("%s's %s limit reached", AgentName(w.Agent), WindowName(w.Name))
		body = "Its agents stop until it resets " + ResetWords(w.ResetsAt, now)
	}
	if machine != "" {
		body += " · read on " + machine
	}
	return push.Payload{Kind: "limits", Title: title, Body: body, Tag: "limits|" + w.Agent + "|" + w.Name, Panel: "usage"}
}

// forecastEarly is how much earlier than its reset the window is forecast to run out, 0 when it is not.
func forecastEarly(w Window) time.Duration {
	if w.Forecast == nil || w.Forecast.RunsOutAt == nil {
		return 0
	}
	return w.ResetsAt.Sub(*w.Forecast.RunsOutAt)
}

// ForecastWarning is the push for a window forecast to run out well before its reset.
func ForecastWarning(w Window, machine string, now time.Time) push.Payload {
	title := fmt.Sprintf("%s's %s limit runs out ≈ %s at this pace", AgentName(w.Agent), WindowName(w.Name), w.Forecast.RunsOutAt.Local().Format("15:04"))
	body := Left(forecastEarly(w)) + " before it resets " + ResetWords(w.ResetsAt, now)
	if machine != "" {
		body += " · read on " + machine
	}
	return push.Payload{Kind: "limits", Title: title, Body: body, Tag: "limits-forecast|" + w.Agent + "|" + w.Name, Panel: "usage"}
}

// ResetWords says when a window resets, in this server's clock: "at 14:00, in 1 h 12 min", "Mon 09:00, in 3 days".
func ResetWords(at, now time.Time) string {
	local := at.Local()
	when := "at " + local.Format("15:04")
	if local.YearDay() != now.Local().YearDay() || local.Year() != now.Local().Year() {
		when = local.Format("Mon 15:04")
	}
	return when + ", in " + Left(at.Sub(now))
}

// Left is a span ahead in words: "40 min", "1 h 12 min", "3 days".
func Left(d time.Duration) string {
	minutes := int(math.Ceil(d.Minutes()))
	switch {
	case minutes < 1:
		return "a moment"
	case minutes < 60:
		return fmt.Sprintf("%d min", minutes)
	case minutes < 48*60:
		if minutes%60 == 0 {
			return fmt.Sprintf("%d h", minutes/60)
		}
		return fmt.Sprintf("%d h %d min", minutes/60, minutes%60)
	}
	return fmt.Sprintf("%d days", int(math.Round(float64(minutes)/(24*60))))
}
