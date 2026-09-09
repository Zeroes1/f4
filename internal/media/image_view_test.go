package media

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func newImageTestScreen(t *testing.T) *vtui.ScreenBuf {
	t.Helper()
	scr := vtui.NewScreenBuf()
	scr.Writer = io.Discard
	scr.AllocBuf(80, 25)
	scr.Graphics().SetProtocol(vtui.GraphicsKitty)
	scr.Graphics().SetCellSize(8, 16)
	return scr
}

func newTestImageView(t *testing.T, w, h int) *ImageView {
	t.Helper()
	iv := &ImageView{
		Path:    "test.png",
		surface: vtui.NewImageSurface(w, h),
		decoder: "test",
		Zoom:    1,
		gfxKey:  "test-key",
	}
	iv.ResizeConsole(80, 25)
	return iv
}

func TestImageViewFitsAndCentres(t *testing.T) {
	scr := newImageTestScreen(t)
	iv := newTestImageView(t, 100, 100)

	p, ok := iv.placementFor(scr)
	if !ok {
		t.Fatal("layout failed")
	}
	// The window has 23 available rows (h=25, minus topbar and bottom border).
	// Available cells: 80x23, cell size 8x16 -> 640x368 pixels.
	// A square image fits to 368x368, which is 46x23 cells, centred horizontally.
	if p.Cols != 46 || p.Rows != 23 {
		t.Errorf("wrong size %dx%d cells", p.Cols, p.Rows)
	}
	if p.Col != 17 || p.Row != 1 {
		t.Errorf("wrong origin %d,%d", p.Col, p.Row)
	}
	if p.SrcW != 0 || p.SrcH != 0 {
		t.Error("a fitting image must not be cropped")
	}
}

func TestImageViewZoomCropsAndPans(t *testing.T) {
	scr := newImageTestScreen(t)
	iv := newTestImageView(t, 100, 100)
	iv.SetZoom(4)

	p, ok := iv.placementFor(scr)
	if !ok {
		t.Fatal("layout failed")
	}
	if p.SrcW <= 0 || p.SrcW >= 100 || p.SrcH <= 0 || p.SrcH >= 100 {
		t.Fatalf("a zoomed image must show only a part of the source, got %dx%d", p.SrcW, p.SrcH)
	}
	if p.SrcX != 0 || p.SrcY != 0 {
		t.Errorf("panning should start at the origin, got %d,%d", p.SrcX, p.SrcY)
	}

	for i := 0; i < 50; i++ {
		iv.Pan(1, 1)
	}
	p, _ = iv.placementFor(scr)
	if p.SrcX+p.SrcW > 100 || p.SrcY+p.SrcH > 100 {
		t.Errorf("panning ran off the image: %d+%d, %d+%d", p.SrcX, p.SrcW, p.SrcY, p.SrcH)
	}
	if p.SrcX == 0 {
		t.Error("panning had no effect")
	}

	for i := 0; i < 50; i++ {
		iv.Pan(-1, -1)
	}
	p, _ = iv.placementFor(scr)
	if p.SrcX != 0 || p.SrcY != 0 {
		t.Errorf("panning back must reach the origin, got %d,%d", p.SrcX, p.SrcY)
	}
}

func TestImageViewZoomOutClearsThePan(t *testing.T) {
	scr := newImageTestScreen(t)
	iv := newTestImageView(t, 100, 100)
	iv.SetZoom(4)
	iv.Pan(1, 1)
	iv.placementFor(scr)

	iv.SetZoom(1)
	p, _ := iv.placementFor(scr)
	if p.SrcX != 0 || p.SrcY != 0 || p.SrcW != 0 {
		t.Errorf("zooming back to fit must drop the crop, got %+v", p)
	}
}

func TestImageViewZoomLimits(t *testing.T) {
	iv := newTestImageView(t, 10, 10)
	for i := 0; i < 200; i++ {
		iv.SetZoom(iv.Zoom * 2)
	}
	if iv.Zoom > imageViewMaxZoom {
		t.Errorf("Zoom escaped its upper limit: %v", iv.Zoom)
	}
	for i := 0; i < 200; i++ {
		iv.SetZoom(iv.Zoom / 2)
	}
	if iv.Zoom < imageViewMinZoom {
		t.Errorf("Zoom escaped its lower limit: %v", iv.Zoom)
	}
}

func TestImageViewKeys(t *testing.T) {
	iv := newTestImageView(t, 100, 100)

	press := func(char rune, vk uint16) bool {
		return iv.ProcessKey(&vtinput.InputEvent{KeyDown: true, Char: char, VirtualKeyCode: vk})
	}

	if !press('+', 0) || iv.Zoom <= 1 {
		t.Error("plus must Zoom in")
	}
	if !press('-', 0) {
		t.Error("minus must be handled")
	}
	if !press('*', 0) || iv.Zoom != 1 {
		t.Errorf("star must reset the Zoom, got %v", iv.Zoom)
	}
	if !press('d', 0) || iv.panX <= 0 {
		t.Error("d must pan")
	}
	if press('~', 0) {
		t.Error("unrelated keys must be left to the rest of the UI")
	}
	if !press(0, vtinput.VK_ESCAPE) || !iv.IsDone() {
		t.Error("escape must close the viewer")
	}
}

func TestImageViewShowDeclaresThePlacement(t *testing.T) {
	scr := newImageTestScreen(t)
	iv := newTestImageView(t, 100, 100)

	scr.Graphics().BeginFrame()
	iv.Show(scr)
	scr.Graphics().EndFrame()

	if scr.Graphics().Len() != 1 {
		t.Fatalf("expected one placement, got %d", scr.Graphics().Len())
	}

	// A frame that is not painted must not leave its picture behind.
	scr.Graphics().BeginFrame()
	scr.Graphics().EndFrame()
	if scr.Graphics().Len() != 0 {
		t.Error("the placement outlived the frame that owned it")
	}
}

// withStubPipeline replaces the application pipeline with one that decodes
// pictures out of thin air, and reports which files it was asked for.
func withStubPipeline(t *testing.T, w, h int) chan string {
	t.Helper()
	asked := make(chan string, 16)

	old := ImagePipe
	t.Cleanup(func() { ImagePipe = old })

	ImagePipe = newTestPipeline(func(ctx context.Context, v vfs.VFS, Path string) (*vtui.ImageSurface, string, error) {
		asked <- Path
		return imageTestSurface(w, h), "stub", nil
	})
	ImagePipe.preview = func(ctx context.Context, v vfs.VFS, Path string) (*vtui.ImageSurface, string, error) {
		return nil, "", errors.New("no thumbnail")
	}
	return asked
}

func TestImageViewWalksItsSiblings(t *testing.T) {
	withStubPipeline(t, 20, 10)

	iv := newTestImageView(t, 100, 100)
	iv.Path = "b.png"
	iv.SetSiblings([]string{"a.png", "b.png", "c.png"}, 1)

	// Decode them all first, so that stepping is answered from the cache.
	for _, name := range []string{"a.png", "b.png", "c.png"} {
		if res := ImagePipe.LoadSync(context.Background(), nil, name); res.Err != nil {
			t.Fatalf("%s: %v", name, res.Err)
		}
	}

	iv.Step(1)
	if iv.Index != 2 || iv.Path != "c.png" {
		t.Fatalf("a step forward went to %d, %q", iv.Index, iv.Path)
	}
	if iv.surface.Width != 20 || iv.surface.Height != 10 {
		t.Errorf("the new picture is not the one on screen: %dx%d", iv.surface.Width, iv.surface.Height)
	}

	// Walking past the end stays on the last picture.
	iv.Step(5)
	if iv.Index != 2 {
		t.Errorf("walking past the end left the list at %d", iv.Index)
	}

	iv.GoTo(0)
	if iv.Index != 0 || iv.Path != "a.png" {
		t.Errorf("Home went to %d, %q", iv.Index, iv.Path)
	}

	// A viewer without siblings has nowhere to step.
	lone := newTestImageView(t, 10, 10)
	lone.Step(1)
	if lone.Path != "test.png" {
		t.Errorf("a lone picture must stay put, got %q", lone.Path)
	}
}

func TestImageViewArrowsWalkWhenThereIsNothingToPan(t *testing.T) {
	withStubPipeline(t, 20, 10)

	iv := newTestImageView(t, 100, 100)
	iv.Path = "b.png"
	iv.SetSiblings([]string{"a.png", "b.png", "c.png"}, 1)
	for _, name := range []string{"a.png", "b.png", "c.png"} {
		if res := ImagePipe.LoadSync(context.Background(), nil, name); res.Err != nil {
			t.Fatalf("%s: %v", name, res.Err)
		}
	}

	press := func(vk uint16) bool {
		return iv.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vk})
	}

	// Nothing has been drawn yet and the picture fits anyway, so the pan
	// range is zero and the arrows walk the directory.
	if !press(vtinput.VK_RIGHT) || iv.Index != 2 {
		t.Fatalf("the right arrow should have stepped forward, index is %d", iv.Index)
	}
	if !press(vtinput.VK_LEFT) || iv.Index != 1 {
		t.Fatalf("the left arrow should have stepped back, index is %d", iv.Index)
	}
	if !press(vtinput.VK_DOWN) || iv.Index != 2 {
		t.Fatalf("the down arrow should have stepped forward, index is %d", iv.Index)
	}
	if iv.panX != 0 || iv.panY != 0 {
		t.Errorf("nothing should have been panned: %v %v", iv.panX, iv.panY)
	}
}

func TestImageViewArrowsPanAZoomedPicture(t *testing.T) {
	withStubPipeline(t, 400, 400)
	scr := newImageTestScreen(t)

	iv := newTestImageView(t, 400, 400)
	iv.SetSiblings([]string{"test.png", "two.png"}, 0)
	if res := ImagePipe.LoadSync(context.Background(), nil, "two.png"); res.Err != nil {
		t.Fatalf("two.png: %v", res.Err)
	}

	iv.SetZoom(8)
	if _, ok := iv.placementFor(scr); !ok {
		t.Fatal("layout failed")
	}
	if iv.panMaxX <= 0 {
		t.Fatalf("a zoomed picture must have room to pan, got %v", iv.panMaxX)
	}

	if !iv.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT}) {
		t.Fatal("the right arrow must be handled")
	}
	if iv.panX <= 0 {
		t.Error("a zoomed picture must be panned, not walked past")
	}
	if iv.Path != "test.png" || iv.Index != 0 {
		t.Errorf("the list must not have moved: %q %d", iv.Path, iv.Index)
	}
}

func TestImageViewInsertPicksAndMovesOn(t *testing.T) {
	withStubPipeline(t, 20, 10)

	iv := newTestImageView(t, 100, 100)
	iv.Path = "a.png"
	iv.SetSiblings([]string{"a.png", "b.png"}, 0)
	for _, name := range []string{"a.png", "b.png"} {
		if res := ImagePipe.LoadSync(context.Background(), nil, name); res.Err != nil {
			t.Fatalf("%s: %v", name, res.Err)
		}
	}

	var told []string
	var states []bool
	iv.OnSelect = func(Path string, on bool) {
		told = append(told, Path)
		states = append(states, on)
	}

	press := func(vk uint16) bool {
		return iv.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vk})
	}

	if !press(vtinput.VK_INSERT) {
		t.Fatal("Insert must be handled")
	}
	if !iv.Selected["a.png"] {
		t.Error("Insert must pick the picture on screen")
	}
	if iv.Path != "b.png" {
		t.Errorf("Insert must then move on, got %q", iv.Path)
	}

	iv.GoTo(0)
	if !press(vtinput.VK_DELETE) {
		t.Fatal("Delete must be handled")
	}
	if iv.Selected["a.png"] {
		t.Error("Delete must unpick the picture on screen")
	}

	if len(told) != 2 || told[0] != "a.png" || told[1] != "a.png" || !states[0] || states[1] {
		t.Errorf("the panel was told %v %v", told, states)
	}
}

func TestImageViewTitleMarksAPickedPicture(t *testing.T) {
	iv := newTestImageView(t, 10, 10)
	if iv.pickMark() != "" || iv.titleAttr() != 0 {
		t.Fatal("an unpicked picture must not be marked")
	}

	iv.SetSelected(iv.Path, true)
	if iv.pickMark() == "" {
		t.Error("a picked picture must be marked in the title")
	}
	if iv.titleAttr() != imageTilePickedAttr {
		t.Error("a picked picture must colour the title bar")
	}

	iv.SetSelected(iv.Path, false)
	if iv.pickMark() != "" || iv.titleAttr() != 0 {
		t.Error("unpicking must take the mark away again")
	}
}

func TestImageViewPrefetchesItsNeighbours(t *testing.T) {
	asked := withStubPipeline(t, 8, 8)

	iv := newTestImageView(t, 100, 100)
	iv.Path = "2.png"
	iv.SetSiblings([]string{"0.png", "1.png", "2.png", "3.png", "4.png"}, 2)

	seen := map[string]bool{}
	for i := 0; i < 4; i++ {
		select {
		case Path := <-asked:
			seen[Path] = true
		case <-time.After(time.Second):
			t.Fatalf("only %d neighbours were prepared: %v", len(seen), seen)
		}
	}
	for _, want := range []string{"1.png", "3.png", "0.png", "4.png"} {
		if !seen[want] {
			t.Errorf("%s was not prepared", want)
		}
	}
	if seen["2.png"] {
		t.Error("the picture on screen is not its own neighbour")
	}
}

func TestImageViewActualSize(t *testing.T) {
	scr := newImageTestScreen(t)
	iv := newTestImageView(t, 100, 100)

	// Fitted, a square picture fills the 23 rows of the window.
	p, _ := iv.placementFor(scr)
	if p.Rows != 23 {
		t.Fatalf("fitted size: %dx%d cells", p.Cols, p.Rows)
	}

	iv.ToggleActualSize()
	p, ok := iv.placementFor(scr)
	if !ok {
		t.Fatal("layout failed")
	}
	// A hundred pixels are thirteen cells of eight and seven cells of
	// sixteen.
	if p.Cols != 13 || p.Rows != 7 {
		t.Errorf("actual size: %dx%d cells", p.Cols, p.Rows)
	}
	if iv.lastScale != 1 {
		t.Errorf("the actual size is a scale of one, got %v", iv.lastScale)
	}

	iv.ToggleActualSize()
	if p, _ = iv.placementFor(scr); p.Rows != 23 {
		t.Errorf("switching back must fit the window again: %dx%d", p.Cols, p.Rows)
	}
}

func TestImageViewFullscreenTakesTheBarRows(t *testing.T) {
	restoreBars(t)
	scr := newImageTestScreen(t)
	iv := newTestImageView(t, 100, 100)

	iv.ProcessKey(&vtinput.InputEvent{KeyDown: true, Char: 'f'})
	if !vtui.FrameManager.HideBars {
		t.Fatal("the key bar is drawn by the manager and has to be told to go away")
	}
	p, ok := iv.placementFor(scr)
	if !ok {
		t.Fatal("layout failed")
	}
	if p.Row != 0 || p.Rows != 25 {
		t.Errorf("without the bars the picture starts at row 0 and fills 25: %d, %d rows", p.Row, p.Rows)
	}
}

func TestImageViewIgnoresStaleResults(t *testing.T) {
	iv := newTestImageView(t, 100, 100)
	gen := iv.loadGen

	// The reader moved on while the decoder was busy.
	iv.loadGen++
	iv.accept(gen, ImageResult{Path: "old.png", Surface: vtui.NewImageSurface(7, 7)})
	if iv.surface.Width != 100 {
		t.Error("a result for a picture nobody is looking at any more must be dropped")
	}

	iv.accept(iv.loadGen, ImageResult{Path: "new.png", Surface: vtui.NewImageSurface(7, 7), Decoder: "stub"})
	if iv.surface.Width != 7 {
		t.Error("the awaited result must be taken")
	}
}
