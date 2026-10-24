package play

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

const videoEvery = 2

func (g *Game) recordFrame(screen *ebiten.Image) error {
	due := g.tick / videoEvery
	if g.frame != nil && due <= g.lastFrame {
		return nil
	}
	if g.frame == nil {
		g.frame = make([]byte, 4*screen.Bounds().Dx()*screen.Bounds().Dy())
		g.lastFrame = due - 1
	}
	screen.ReadPixels(g.frame)
	for ; g.lastFrame < due; g.lastFrame++ {
		if n, err := g.opts.Video.Write(g.frame); err != nil {
			return err
		} else if n != len(g.frame) {
			return io.ErrShortWrite
		}
	}
	return nil
}

func Record(ctx context.Context, opts Options, out string) (err error) {
	if opts.Script == "" && opts.Autopilot == nil && opts.Replay == "" {
		return &ArgumentError{Err: errors.New("record: a script, the autopilot or a replay is needed")}
	}
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("record: ffmpeg must be on PATH; see https://github.com/nao1215/gozen/blob/main/docs/development/graphics.md: %w", err)
	}
	db, err := data.Load(opts.FS)
	if err != nil {
		return err
	}
	w, h := db.Game.Screen.Width, db.Game.Screen.Height
	tmp, err := os.MkdirTemp("", "gozen-record")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(tmp)) }()
	audioPath := filepath.Join(tmp, "audio.f32")
	videoPath := filepath.Join(tmp, "video.mp4")
	audio, err := os.Create(audioPath) //nolint:gosec // audioPath is a fixed basename in this run's private temporary directory.
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, audio.Close()) }()
	//nolint:gosec // The executable comes from LookPath; arguments are separate and no shell is used.
	enc := exec.CommandContext(ctx, ff, "-loglevel", "error", "-y",
		"-f", "rawvideo", "-pix_fmt", "rgba", "-s", fmt.Sprintf("%dx%d", w, h), "-r", strconv.Itoa(60/videoEvery), "-i", "-",
		"-c:v", "libx264", "-preset", "medium", "-crf", "23", "-pix_fmt", "yuv420p", videoPath)
	enc.Stderr = os.Stderr
	pipe, err := enc.StdinPipe()
	if err != nil {
		return err
	}
	if err := enc.Start(); err != nil {
		return err
	}
	opts.Video, opts.Audio = pipe, audio
	runErr := Run(ctx, opts)
	if err := errors.Join(pipe.Close(), enc.Wait()); err != nil {
		return fmt.Errorf("record: %w", err)
	}
	if runErr != nil {
		return runErr
	}
	//nolint:gosec // The executable comes from LookPath; arguments are separate and no shell is used.
	mux := exec.CommandContext(ctx, ff, "-loglevel", "error", "-y", "-i", videoPath,
		"-f", "f32le", "-ar", "44100", "-ac", "2", "-i", audioPath,
		"-c:v", "copy", "-c:a", "aac", "-b:a", "160k", "-shortest", "-movflags", "+faststart", out)
	mux.Stderr = os.Stderr
	return mux.Run()
}
