package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"

	"github.com/gammazero/workerpool"
	"github.com/gocarina/gocsv"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
	"github.com/zmb3/spotify/v2"
	"go.uber.org/zap"
)

var likedCmd = &cobra.Command{
	Use:   "liked",
	Short: "Manage liked tracks",
}

func init() {
	likedListCmd.Flags().StringVar(&likedListConf.json, "json", "", "output playlist list as json")
	likedListCmd.Flags().StringVar(&likedListConf.csv, "csv", "", "output playlist list as csv")

	purgeLikedCmd.Flags().StringVar(&likedPurgeConf.csv, "csv", "", "input csv to purge from")
	purgeLikedCmd.Flags().BoolVar(&likedPurgeConf.dryrun, "dryrun", false, "dryrun")

	likedCmd.AddCommand(likedListCmd)
	likedCmd.AddCommand(purgeLikedCmd)

	rootCmd.AddCommand(likedCmd)
}

var likedListConf = struct {
	json string
	csv  string
}{}

var likedPurgeConf = struct {
	csv    string
	dryrun bool
}{}

type trackRow struct {
	ID      string `csv:"id"`
	Name    string `csv:"name"`
	Artists string `csv:"artists"`
	Delete  bool   `csv:"delete,omitempty"`
}

var likedListCmd = &cobra.Command{
	Use:   "list",
	Short: "List liked tracks",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		zap.L().Info("listing liked tracks")
		spClient, err := getClient(ctx)
		if err != nil {
			return err
		}

		likedRes, err := spClient.CurrentUsersTracks(ctx, spotify.Limit(50))
		if err != nil {
			return err
		}

		tracks := likedRes.Tracks

		for {
			err = spClient.NextPage(ctx, likedRes)
			if err == spotify.ErrNoMorePages {
				break
			}

			if err != nil {
				return err
			}

			tracks = append(tracks, likedRes.Tracks...)
		}

		zap.L().Info("found", zap.Int("tracks", len(tracks)))

		if likedListConf.json != "" {
			bytes, err := json.Marshal(tracks)
			if err != nil {
				return err
			}

			err = os.WriteFile(likedListConf.json, bytes, os.ModePerm)
			if err != nil {
				return err
			}
		}

		if likedListConf.csv != "" {
			rows := make([]trackRow, len(tracks))
			for i, track := range tracks {
				artists := make([]string, len(track.Artists))
				for j, artist := range track.Artists {
					artists[j] = artist.Name
				}

				sort.Strings(artists)

				rows[i] = trackRow{
					ID:      string(track.ID),
					Name:    track.Name,
					Artists: strings.Join(artists, "; "),
				}
			}

			f, err := os.OpenFile(likedListConf.csv, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.ModePerm)
			if err != nil {
				return err
			}

			defer f.Close()

			err = gocsv.MarshalFile(rows, f)
			if err != nil {
				return err
			}
		}

		return nil
	},
}

var purgeLikedCmd = &cobra.Command{
	Use:   "purge",
	Short: "Purge liked tracks",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		if likedPurgeConf.csv == "" {
			return errors.New("--csv argument is required")
		}

		zap.L().Info("purging liked tracks")
		spClient, err := getClient(ctx)
		if err != nil {
			return err
		}

		f, err := os.Open(likedPurgeConf.csv)
		if err != nil {
			return err
		}

		defer f.Close()

		rows := []trackRow{}
		err = gocsv.UnmarshalFile(f, &rows)
		if err != nil {
			return err
		}

		toPurge := make([]spotify.ID, 0)
		for _, row := range rows {
			if row.Delete {
				toPurge = append(toPurge, spotify.ID(row.ID))
			}
		}

		zap.L().Info("found", zap.Int("tracks", len(rows)), zap.Int("to-purge", len(toPurge)))

		if likedPurgeConf.dryrun {
			zap.L().Info("dryrun, not purging")
			return nil
		}

		batches := make([][]spotify.ID, 0)

		for i := 0; i < len(toPurge); i += 50 {
			end := i + 50
			if end > len(toPurge) {
				end = len(toPurge)
			}

			batches = append(batches, toPurge[i:end])
		}

		bar := progressbar.Default(int64(len(batches)))

		wp := workerpool.New(5)

		for _, batch := range batches {
			batch := batch
			wp.Submit(func() {
				// nolint
				defer bar.Add(1)
				err := spClient.RemoveTracksFromLibrary(ctx, batch...)
				if err != nil {
					zap.L().Error("failed to remove tracks", zap.Error(err))
				}
			})
		}

		wp.StopWait()

		return nil
	},
}
