package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/Sora378/codingplantracker/internal/config"
	"github.com/Sora378/codingplantracker/internal/db"
	"github.com/Sora378/codingplantracker/internal/models"
	"github.com/spf13/cobra"
)

var historyDays int

type dayData struct {
	date        string
	snapshots   []models.UsageSnapshot
	windowMax   int
	windowMin   int
	windowTotal int
	weeklyMax   int
	weeklyMin   int
	weeklyTotal int
	hourlyData  map[int][]int
}

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "View usage history",
	Long:  `View historical usage snapshots from the local database.`,
	RunE:  runHistory,
}

func init() {
	historyCmd.Flags().IntVarP(&historyDays, "days", "d", 7, "Number of days to show (default 7, max 90)")
	rootCmd.AddCommand(historyCmd)
}

func runHistory(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	if !cfg.IsLoggedIn() {
		fmt.Println("  Not logged in. Run 'cpq --cli' and login first.")
		return nil
	}

	database, err := db.New(dbPath(cfg))
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	user, err := database.GetLatestUser()
	if err != nil {
		return fmt.Errorf("no user found in database: %w", err)
	}

	snapshots, err := database.GetUsageSnapshots(user.ID, historyDays*24*2) // more snapshots for trends
	if err != nil {
		return fmt.Errorf("failed to get history: %w", err)
	}

	if len(snapshots) == 0 {
		fmt.Println()
		fmt.Println("  No usage history yet.")
		fmt.Println("  History is recorded automatically while the app is running.")
		return nil
	}

	days := make(map[string]*dayData)
	var dayOrder []string

	for _, s := range snapshots {
		day := s.CreatedAt.Format("2006-01-02")
		if _, ok := days[day]; !ok {
			days[day] = &dayData{
				date:       day,
				hourlyData: make(map[int][]int),
			}
			dayOrder = append(dayOrder, day)
		}
		d := days[day]
		d.snapshots = append(d.snapshots, s)

		if s.WindowUsed > d.windowMax || d.windowMax == 0 {
			d.windowMax = s.WindowUsed
		}
		if s.WindowRemaining < d.windowMin || d.windowMin == 0 {
			d.windowMin = s.WindowRemaining
		}
		d.windowTotal += s.WindowUsed

		if s.WeeklyUsed > d.weeklyMax || d.weeklyMax == 0 {
			d.weeklyMax = s.WeeklyUsed
		}
		if s.WeeklyRemaining < d.weeklyMin || d.weeklyMin == 0 {
			d.weeklyMin = s.WeeklyRemaining
		}
		d.weeklyTotal += s.WeeklyUsed

		// Group by hour
		hour := s.CreatedAt.Hour()
		d.hourlyData[hour] = append(d.hourlyData[hour], s.WindowUsed)
	}

	fmt.Println()
	fmt.Println("  ═════════════════════════════════════════════════════════════")
	fmt.Println("                    Usage History")
	fmt.Println("  ═════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Printf("  Showing last %d days (%d snapshots)\n\n", len(dayOrder), len(snapshots))

	weekdays := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}

	// Print each day with trend bars
	for i := len(dayOrder) - 1; i >= 0; i-- {
		day := dayOrder[i]
		d := days[day]
		t, _ := time.Parse("2006-01-02", day)
		weekday := weekdays[t.Weekday()]

		avgWindow := 0
		if len(d.snapshots) > 0 {
			avgWindow = d.windowTotal / len(d.snapshots)
		}

		fmt.Printf("  ┌─────────────────────────────────────────────────────────┐\n")
		fmt.Printf("  │ %s %s  (avg: %d req/5h)                    │\n", weekday, day, avgWindow)
		fmt.Printf("  ├─────────────────────────────────────────────────────────┤\n")

		// Trend bar chart by hour (show all 24 hours, even if no data)
		hourBars := newBarChart(d.hourlyData, d.windowMax)
		fmt.Printf("  │ 5H Window Trend: %s│\n", hourBars)

		// Summary stats
		windowRange := fmt.Sprintf("%d-%d", d.windowMin, d.windowMax)
		if d.windowMin == d.windowMax {
			windowRange = fmt.Sprintf("%d", d.windowMax)
		}
		weeklyRange := fmt.Sprintf("%d-%d", d.weeklyMin, d.weeklyMax)
		if d.weeklyMin == d.weeklyMax {
			weeklyRange = fmt.Sprintf("%d", d.weeklyMax)
		}

		fmt.Printf("  │  Range: %-12s  Weekly: %-12s           │\n", windowRange, weeklyRange)

		if len(d.snapshots) > 0 {
			first := d.snapshots[len(d.snapshots)-1]
			last := d.snapshots[0]
			fmt.Printf("  │  Snapshots: %d  (first: %s, last: %s)              │\n",
				len(d.snapshots),
				first.CreatedAt.Format("15:04"),
				last.CreatedAt.Format("15:04"))
		}

		fmt.Printf("  └─────────────────────────────────────────────────────────┘\n")
	}

	// Overall trend summary
	fmt.Println()
	overallTrend(days, dayOrder)

	fmt.Println()
	fmt.Printf("  Tip: Run with -d 30 to see more history\n")

	return nil
}

func newBarChart(hourlyData map[int][]int, maxVal int) string {
	var bars []string
	for hour := 0; hour < 24; hour++ {
		vals, ok := hourlyData[hour]
		if !ok || len(vals) == 0 {
			bars = append(bars, " ")
			continue
		}
		// Average for this hour
		sum := 0
		for _, v := range vals {
			sum += v
		}
		avg := sum / len(vals)

		if maxVal > 0 {
			pct := float64(avg) / float64(maxVal)
			bars = append(bars, barChar(pct))
		} else {
			bars = append(bars, " ")
		}
	}
	return strings.Join(bars, "")
}

func barChar(pct float64) string {
	switch {
	case pct >= 0.9:
		return "█"
	case pct >= 0.7:
		return "▓"
	case pct >= 0.5:
		return "▒"
	case pct >= 0.3:
		return "░"
	case pct > 0:
		return "▁"
	default:
		return " "
	}
}

func overallTrend(days map[string]*dayData, dayOrder []string) {
	fmt.Println("  Weekly Summary:")
	fmt.Println("  ─────────────────────────────────────────────────────────")

	// Calculate last 7 days summary
	var totalSnapshots int
	var totalWindow int
	var maxWindow int
	var daysCount int

	for i := len(dayOrder) - 1; i >= 0 && i >= len(dayOrder)-7; i-- {
		daysCount++
		d := days[dayOrder[i]]
		totalSnapshots += len(d.snapshots)
		totalWindow += d.windowTotal
		if d.windowMax > maxWindow {
			maxWindow = d.windowMax
		}
	}

	avgDaily := 0
	if daysCount > 0 {
		avgDaily = totalWindow / daysCount
	}

	// Simple bar for overall
	barLen := 0
	if maxWindow > 0 && totalWindow > 0 {
		barLen = (totalWindow / maxWindow) * 50 / daysCount
		if barLen > 50 {
			barLen = 50
		}
	}
	bar := strings.Repeat("█", barLen)

	fmt.Printf("  Last %d days: %d snapshots, avg %d req/day\n", daysCount, totalSnapshots, avgDaily)
	fmt.Printf("  [%s] %d total\n", bar, totalWindow)
}

func loadConfig() (*config.Config, error) {
	cfg, err := config.LoadFrom(cfgFile)
	if err != nil {
		cfg = config.DefaultConfigFrom(cfgFile)
	}
	return cfg, nil
}

func dbPath(cfg *config.Config) string {
	return cfg.ConfigDir() + "/usage.db"
}
