package editor

//
//// EditMode can be either NormalEditMode, EditBeforeCreateMode or ApplyEditMode
//type EditMode string
//
//const (
//	// NormalEditMode is an edit mode
//	NormalEditMode EditMode = "normal_mode"
//
//	// EditBeforeCreateMode is an edit mode
//	EditBeforeCreateMode EditMode = "edit_before_create_mode"
//
//	// ApplyEditMode is an edit mode
//	ApplyEditMode EditMode = "edit_last_applied_mode"
//)
//
//type EditOptions struct {
//	EditMode           EditMode
//	WindowsLineEndings bool
//	galaxycfg.IOStreams
//}
//
//func NewEditOptions(editMode EditMode, ioStreams galaxycfg.IOStreams) *EditOptions {
//	return &EditOptions{
//		EditMode:           editMode,
//		WindowsLineEndings: goruntime.GOOS == "windows",
//		IOStreams:          ioStreams,
//	}
//}
//
//func (that *EditOptions) Run() error {
//	edit := NewDefaultEditor(editorEnvs())
//	// generate the file to edit
//	buf := &bytes.Buffer{}
//	var w io.Writer = buf
//	if that.WindowsLineEndings {
//		w = crlf.NewCRLFWriter(w)
//	}
//	var (
//		edited = []byte{}
//		file   string
//		err    error
//	)
//	containsError := false
//	buf.Write(cmdutil.ManualStrip(edited))
//	// launch the editor
//	editedDiff := edited
//	edited, file, err = edit.LaunchTempFile(fmt.Sprintf("%s-edit-", filepath.Base(os.Args[0])), o.editPrinterOptions.ext, buf)
//	if err != nil {
//		return preservedFile(err, results.file, that.ErrOut)
//	}
//	// If we're retrying the loop because of an error, and no change was made in the file, short-circuit
//	if containsError && bytes.Equal(cmdutil.StripComments(editedDiff), cmdutil.StripComments(edited)) {
//		return preservedFile(fmt.Errorf("%s", "Edit cancelled, no valid changes were saved."), file, o.ErrOut)
//	}
//}
//
//// preservedFile writes out a message about the provided file if it exists to the
//// provided output stream when an error happens. Used to notify the user where
//// their updates were preserved.
//func preservedFile(err error, path string, out io.Writer) error {
//	if len(path) > 0 {
//		if _, err := os.Stat(path); !os.IsNotExist(err) {
//			fmt.Fprintf(out, "A copy of your changes has been stored to %q\n", path)
//		}
//	}
//	return err
//}

// editorEnvs returns an ordered list of env vars to check for editor preferences.
func EditorEnvs() []string {
	return []string{
		"GALAXY_EDITOR",
		"EDITOR",
	}
}
