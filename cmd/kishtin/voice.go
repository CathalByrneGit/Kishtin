package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/doordash-oss/oapi-codegen-dd/v3/pkg/runtime"
	"github.com/gabriel-vasile/mimetype"
	"github.com/spf13/cobra"

	"github.com/cathalbyrnegit/kishtin/internal/client"
)

func printReply(w io.Writer, r client.VoiceReply) {
	fmt.Fprintf(w, "Heard: %s\n", r.Heard)
	fmt.Fprintln(w, r.Say)
	if len(r.Options) > 0 {
		fmt.Fprintf(w, "Answer with: kishtin say %q (or one of: %s)\n", r.Options[0], strings.Join(r.Options, ", "))
	}
	if deref(r.Confirm) {
		fmt.Fprintln(w, "Answer with: kishtin say confirm (or kishtin say cancel)")
	}
}

func (c *cli) sayCmd() *cobra.Command {
	var audio string
	var handsFree bool
	cmd := &cobra.Command{
		Use:   "say [COMMAND...]",
		Short: "Run a voice command, typed or from an audio file",
		Long: `Run a voice command through the same grammar as push-to-talk, such as
  kishtin say fire table 4 mains
  kishtin say walk-in fridge 3.4 degrees
  kishtin say --audio command.wav

When the reply asks a question, the next "kishtin say" answers it.
Audio is transcribed by the server's local speech-to-text sidecar.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			var res *client.VoiceReply
			switch {
			case audio != "":
				if len(args) > 0 {
					return fmt.Errorf("give a command or --audio, not both")
				}
				data, err := os.ReadFile(audio)
				if err != nil {
					return err
				}
				var f runtime.File
				f.InitFromBytes(data, filepath.Base(audio))
				ct := mimetype.Detect(data).String()
				opts := &client.VoiceAudioRequestOptions{Body: &f, Header: &client.VoiceAudioHeaders{ContentType: &ct}}
				if handsFree {
					opts.Query = &client.VoiceAudioQuery{HandsFree: &handsFree}
				}
				res, err = c.api.VoiceAudio(ctx, opts)
				if err != nil {
					return apiErr(err)
				}
			case len(args) > 0:
				var err error
				res, err = c.api.VoiceCommand(ctx, &client.VoiceCommandRequestOptions{Body: &client.VoiceCommandBody{Text: strings.Join(args, " ")}})
				if err != nil {
					return apiErr(err)
				}
			default:
				return fmt.Errorf("say what: kishtin say fire table 4 mains")
			}
			return c.emit(res, func(w io.Writer) {
				if res.Outcome == "ignored" {
					fmt.Fprintln(w, "No wake word heard: ignored, and not logged.")
					return
				}
				printReply(w, *res)
			})
		},
	}
	cmd.Flags().StringVar(&audio, "audio", "", "an audio file to transcribe (WAV, WebM, Ogg)")
	cmd.Flags().BoolVar(&handsFree, "hands-free", false, "with --audio: the recording must start with the wake word, as in hands-free listening")
	return cmd
}

func (c *cli) voiceCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "voice", Short: "Voice control: status and log"}
	status := &cobra.Command{
		Use: "status", Short: "Show whether spoken commands and replies are set up", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := c.api.VoiceStatus(cmd.Context())
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) {
				yes := map[bool]string{true: "set up", false: "not set up (speech.%s in kishtin.toml)"}
				fmt.Fprintf(w, "Speech-to-text: "+yes[res.Listen]+"\n", "stt_url")
				fmt.Fprintf(w, "Text-to-speech: "+yes[res.Speak]+"\n", "tts_url")
				if q := deref(res.Waiting); q != "" {
					fmt.Fprintln(w, "Waiting for your answer:", q)
				}
			})
		},
	}
	var limit int64
	log := &cobra.Command{
		Use: "log", Short: "List recent voice commands: heard, meaning, outcome, reply", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := c.api.VoiceLog(cmd.Context(), &client.VoiceLogRequestOptions{Query: &client.VoiceLogQuery{Limit: &limit}})
			if err != nil {
				return apiErr(err)
			}
			return c.emit(res, func(w io.Writer) {
				var rows [][]string
				for _, e := range res.Entries {
					rows = append(rows, []string{hhmm(e.At), e.UserName, e.Source, e.Transcript, e.Outcome, e.Reply})
				}
				table(w, "TIME\tWHO\tHOW\tHEARD\tOUTCOME\tREPLY", rows)
			})
		},
	}
	log.Flags().Int64Var(&limit, "limit", 50, "how many, newest first")
	cmd.AddCommand(status, log)
	return cmd
}
