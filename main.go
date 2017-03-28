package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/eduncan911/safeincloud"
	"github.com/golang/glog"
	"github.com/pkg/errors"
)

var (
	dbFile             string
	defaultFolder      string
	priorityFoldersRaw string
	priorityFolders    []string

	importedSites []site
	importedNotes []note
	extraFormat   = `%s: %s

`
)

func main() {
	flag.Parse()
	if dbFile == "" {
		flag.Usage()
		os.Exit(0)
	}
	if priorityFoldersRaw != "" {
		priorityFolders = strings.Split(priorityFoldersRaw, ",")
	}

	// parse the SafeInCloud exported XML
	db, err := safeincloud.ParseFile(dbFile)
	if err != nil {
		glog.Error(err)
		os.Exit(10)
	}

	// iterate over the SIC cards and parse
	imported, deleted, skipped := 0, 0, 0
	for _, c := range db.Cards {
		if c.Deleted {
			glog.Infoln("skipping deleted card", c.ID, c.Title)
			deleted++
			continue
		}
		if c.Template {
			glog.Infoln("skipping template", c.ID, c.Title)
			skipped++
			continue
		}

		if err := parse(db, c, priorityFolders, defaultFolder); err != nil {
			glog.Errorln(err)
			os.Exit(11)
		}
		imported++
	}

	// export all to csvs
	if err := writeSitesCSV(importedSites); err != nil {
		glog.Errorln("writeSitesCSV error:", err)
		os.Exit(12)
	}
	if err := writeSecureNotesCSV(importedNotes); err != nil {
		glog.Errorln("writeSitesCSV error:", err)
		os.Exit(13)
	}

	glog.Infoln("Total Imported, Deleted, Skipped:", imported, deleted, skipped)
}

// parse determines the type of card to import.
//
// to parse "sites" for LastPass, they require:
//	- URL (sic Website type)
//	- Username (sic Login type)
//	- Password (sic Password type)
//	- Name (sic Title)
//
// the logic here is that we are going to be looking at the
// SafeInCloud field types and if they ALL exist, import it as a site.
// else, treat the card as a Secure Note (which means no auto-login).
//
// notice that all fields will be included in the Notes section, just in case
// some fields are missing with the multiple entries.
//
// O: if a card has an empty title, but it has everything else, we'll take the
// website field and make that the Title of the site to import, keeping it
// as a Site and not a Secure Note.
//
// O: if the card has multiple Login types, we'll treat each login
// as a separate site entry at LastPass as this would allow for multiple
// options to signin.
//
// O: if the card has multiple Login types, besides treating them as multiple
// sites as mentioned above, we'll also be using the login & pass SEQUENTIALLY
// found in the fields, in the order they are from sic.  they must be in the
// correct order for the LP site to work properly with multiple logins like this.
//
// O == Opinionated Logic
func parse(db *safeincloud.Database, c safeincloud.Card, pf []string, df string) error {
	glog.V(5).Infoln(c.ID, c.Title, "being parsed.")
	var importedSite bool
	// loop the fields, looking for login, password and website SIC Types
	for i, f := range c.Fields {

		if f.FieldType == "login" && f.Value != "" {
			glog.V(5).Infoln(c.ID, c.Title, "found login.")
			login := f.Value

			var pass, url string
			for _, fi := range c.Fields[i:] {
				if fi.FieldType == "password" && fi.Value != "" {
					glog.V(5).Infoln(c.ID, c.Title, "found password.")
					pass = fi.Value
					break // break on the 1ST password found, don't keep loopin
				}
			}
			for _, fi := range c.Fields[i:] {
				if fi.FieldType == "website" && fi.Value != "" {
					glog.V(5).Infoln(c.ID, c.Title, "found website.")
					url = fi.Value
					break // break on the 1ST website found, don't keep loopin
				}
			}

			if pass == "" || url == "" {
				glog.V(3).Infoln(c.ID, c.Title, "missing password or website value(s).")
				continue
			}

			title := c.Title
			if title == "" {
				glog.V(5).Infoln(c.ID, c.Title, "title was empty, attemping to use website as title.")
				title = url
				title = strings.Replace(title, "http://", "", -1)
				title = strings.Replace(title, "https://", "", -1)
			}
			if title == "" {
				glog.V(3).Infoln(c.ID, c.Title, "missing title.")
				continue
			}

			// import as a LastPass site!
			if err := importSite(db, c, pf, df, title, login, pass, url); err != nil {
				return errors.Wrap(err, "importSite returned error")
			}
			importedSite = true
		}
	}
	if importedSite {
		glog.V(5).Infoln(c.ID, c.Title, "has been imported as site.")
		return nil
	}

	// since we haven't imported anything, we'll treat it as a Secure Note
	// going forward.
	if err := importSecureNote(db, c, pf, df); err != nil {
		return errors.Wrap(err, "importSecureNote returned error")
	}
	return nil
}

// site defines a LastPass site to import.
//
// Sites require all of the following at a minimal: URL, Username, Password, Name.
type site struct {
	URL      string `csv:"url"`
	Type     string `csv:"type"`
	Username string `csv:"username"`
	Password string `csv:"password"`
	Hostname string `csv:"hostname"`
	Extra    string `csv:"extra"`
	Name     string `csv:"name"`
	Grouping string `csv:"grouping"`
	Fav      string `csv:"fav"` // ?
}

// importSite assumes the safeincloud.Card has been validated.  It will then
// generate a card entry and store it in the global importedSites variable.
func importSite(db *safeincloud.Database, c safeincloud.Card, pf []string, df, title, login, pass, url string) error {
	s := site{
		Name:     title,
		URL:      url,
		Username: login,
		Password: pass,
	}
	if c.Star {
		glog.V(5).Infoln(c.ID, title, "found favorite.")
		s.Fav = "1"
	}
	s.Grouping = primaryCardLabel(db, c, pf, df)
	glog.Infoln("importing Website", c.ID, title, "->", s.Grouping)

	// build up the Extra section to comprise of the entire card.
	for _, f := range c.Fields {
		// we'll exclue what we already have above.
		if f.Value == url ||
			f.Value == login ||
			f.Value == pass {
			continue
		}
		s.Extra = s.Extra + fmt.Sprintf(extraFormat, f.Name, f.Value)
	}
	s.Extra = s.Extra + c.Notes

	// add the original Labels this card was part of
	labels := strings.Join(cardLabels(db, c), ", ")
	if len(labels) > 0 {
		s.Extra = s.Extra + `

Labels: ` + labels
	}

	// dump attachments for manual imports
	if err := extractAttachments(c, title); err != nil {
		return errors.Wrap(err, "extractAttachments returned error")
	}

	importedSites = append(importedSites, s)
	return nil
}

// note defines a Secure Note at LastPass.
//
// * URL must be set to "http://sn" for all entries.
// * Username and Password must be BLANK for all entries, except for Servers.
type note struct {
	URL      string `csv:"url"`
	Username string `csv:"username"`
	Password string `csv:"password"`
	Extra    string `csv:"extra"`
	Name     string `csv:"name"`
	Grouping string `csv:"grouping"`
	Fav      string `csv:"fav"`
}

// importSecureNote assumes nothing.  It will attempt to take as much info
// as possible from the SIC card and create a SecureNote for LastPass.
func importSecureNote(db *safeincloud.Database, c safeincloud.Card, pf []string, df string) error {
	title := c.Title
	if title == "" {
		title = "SecureNote " + c.ID
	}
	n := note{
		URL:      "http://sn", // must be set to this
		Name:     title,
		Username: "", // must be blank
		Password: "", // must be blank
	}
	if c.Star {
		glog.V(5).Infoln(c.ID, title, "found favorite.")
		n.Fav = "1"
	}
	n.Grouping = primaryCardLabel(db, c, pf, df)
	glog.Infoln("importing Secure Note", c.ID, title, "->", n.Grouping)

	// build up the Extra section to comprise of the entire card.
	//
	// prefix with the expected NoteType, based on the Primary Grouping.
	var prefix string
	switch n.Grouping {
	case "Credit Cards":
		prefix = "NoteType:Credit Card"
	case "Banking":
		prefix = "NoteType:Bank Account"
	case "Databases":
		prefix = "NoteType:Database"
	case "Licenses":
		prefix = "NoteType:Driver's License"
	case "Insurance":
		prefix = "NoteType:Insurance"
	case "Membership":
		prefix = "NoteType:Membership"
	case "Passport":
		prefix = "NoteType:Passport"
	case "Servers":
		prefix = "NoteType:Server"
	case "Software":
		prefix = "NoteType:Software License"
	}

	if prefix != "" {
		n.Extra = prefix + `

` // LastPass expects a line break
	}

	// NOTE: it's best to go back into SafeInCloud and massage each FieldName
	// to match that of LastPass' expected field name.
	//
	// see their import format: https://helpdesk.lastpass.com/importing-from-other-password-managers/
	//
	// For example, for Credit Cards, you want to edit each card
	// in SafeInCloud to change "Owner" to "Name on Card", "CVV" to "Security Code"
	// and so on.
	for _, f := range c.Fields {
		n.Extra = n.Extra + fmt.Sprintf(extraFormat, f.Name, f.Value)
	}
	n.Extra = n.Extra + c.Notes

	// add the original Labels this card was part of
	labels := strings.Join(cardLabels(db, c), ", ")
	if len(labels) > 0 {
		n.Extra = n.Extra + `

Labels: ` + labels
	}

	// dump attachments for manual imports
	if err := extractAttachments(c, title); err != nil {
		return errors.Wrap(err, "extractAttachments returned error")
	}

	importedNotes = append(importedNotes, n)
	return nil
}

// extractAttachments takes a Card input and saves all attachments to disk.
func extractAttachments(c safeincloud.Card, title string) error {
	for i, file := range c.Files {
		name := title + "_" + strconv.Itoa(i) + "_" + file.Name
		if err := dumpfile(name, file.Value); err != nil {
			return errors.Wrap(err, "dumpfile for files returned error")
		}
		glog.Warningln("  -", c.ID, title, "file attachment saved to", name)
	}
	for i, image := range c.Images {
		// SafeInCloud forces all images to JPEG and compressed to 80%.
		// this kind of screws up all sorts of images and filenames.  Therefore,
		// all we can do is name the image via the title as a .jpg extension.
		name := title + "_" + strconv.Itoa(i) + ".jpg"
		if err := dumpfile(name, image.Value); err != nil {
			return errors.Wrap(err, "dumpfile for images returned error")
		}
		glog.Warningln("  -", c.ID, title, "image attachment saved to", name)
	}
	return nil
}

// dumpfile will dump the binary contents of data to filename inside of a
// directory called "attachments" where the utility is run.
func dumpfile(filename string, data []byte) error {
	cfilename := url.QueryEscape(filename)
	cfilename = strings.Replace(cfilename, "%20", " ", -1)
	dir := "attachments"
	if err := os.MkdirAll(dir, 0700); err != nil {
		return errors.Wrap(err, "os.MkdirAll returned error")
	}
	fullpath := dir + string(os.PathSeparator) + cfilename
	if err := ioutil.WriteFile(fullpath, data, 0700); err != nil {
		return errors.Wrap(err, "ioutil.WriteFile returned error")
	}
	return nil
}

// primaryCardLabel looks at all the labels for the card and determines which
// label will become the "Folder" to import it into LastPass.
//
// LastPass only supports a single Folder or Group for sites and notes.
// LastPass does not have a concept of Tags or Labels.  Therefore, we need some
// logic to determine which label to sort the site/note into.
//
// This method looks at the CLI option of "-p" to determine what label will be
// assigned the primary folder.  It does this in order assigned to this param
// by iterating the primary folder list to see if the Card is assigned one of
// the labels.  The first match wins.
//
// You most likely want to set the strictest "Google" first and leave more
// generic labels "Banking,Personal" last.  That way, your preferred label is
// used first.
//
// Lastly, if the card's label is not in the PriorityFolders slice then we'll
// just use the first one we find - prefixed with the specified
// "DefaultFolder - " to make it easier to sort.
func primaryCardLabel(db *safeincloud.Database, c safeincloud.Card, pf []string, df string) string {
	labels := cardLabels(db, c)
	if len(labels) == 0 {
		return df
	}

	// loop over the PriorityFolders and look for any card labels that match.
	// first match wins.
	for _, f := range pf {
		for _, l := range labels {
			if strings.EqualFold(f, l) {
				return f
			}
		}
	}

	// if no labels matched, just pick the first one prefix it with the
	// default folder (df).
	return df + " - " + labels[0]
}

// cardLabels takes the Card.LabelIDs and finds their corresponding string
// name in the SIC database and returns the string labels in a slice.
func cardLabels(db *safeincloud.Database, c safeincloud.Card) []string {
	var labels []string
	for _, id := range c.LabelIDs {
		for _, label := range db.Labels {
			if label.ID == id {
				labels = append(labels, label.Name)
			}
		}
	}
	return labels
}

// writeSitesCSV takes a list of sites and writes them to a csv.
func writeSitesCSV(sites []site) error {
	f, err := os.Create("lastpass_sites.csv")
	if err != nil {
		return errors.Wrap(err, "os.Create error")
	}
	defer f.Close()
	if len(sites) == 0 {
		return nil
	}

	w := csv.NewWriter(f)
	headers := csvHeaders(sites[0])
	if err := w.Write(headers); err != nil {
		return errors.Wrap(err, "writer.Write Headers error")
	}
	for _, s := range sites {
		row := csvSlice(s)
		if err := w.Write(row); err != nil {
			return errors.Wrap(err, "writer.Write Entry error")
		}
	}
	defer w.Flush()
	return nil
}

// writeSecureNotesCSV takes a list of notes and writes them to a csv.
func writeSecureNotesCSV(notes []note) error {
	f, err := os.Create("lastpass_notes.csv")
	if err != nil {
		return errors.Wrap(err, "os.Create error")
	}
	defer f.Close()
	if len(notes) == 0 {
		return nil
	}

	w := csv.NewWriter(f)
	headers := csvHeaders(notes[0])
	if err := w.Write(headers); err != nil {
		return errors.Wrap(err, "writer.Write Headers error")
	}
	for _, s := range notes {
		row := csvSlice(s)
		if err := w.Write(row); err != nil {
			return errors.Wrap(err, "writer.Write Entry error")
		}
	}
	defer w.Flush()
	return nil
}

// csvHeaders evaluates a struct's tags and returns the csv headers.
func csvHeaders(v interface{}) []string {
	var results []string
	value := reflect.ValueOf(v)
	for i := 0; i < value.NumField(); i++ {
		t := value.Type().Field(i).Tag
		results = append(results, t.Get("csv"))
	}
	return results
}

// csvSlice evaluates a struct's fields and returns its values as strings.
func csvSlice(v interface{}) []string {
	var results []string
	value := reflect.ValueOf(v)
	for i := 0; i < value.NumField(); i++ {
		f := value.Field(i)
		results = append(results, f.String())
	}
	return results
}

// init sets the the global flag and variables.
//
// For the dbFile, it takes the first argument passed into the program.  If
// that argument is prefixed with -,help,?,/? it will be skipped.
func init() {
	flag.Usage = func() {
		script := os.Args[0]
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", script)
		fmt.Fprintf(os.Stderr, "  %s -db /path/to/SafeInCloud_Export.xml [options]\n", script)
		fmt.Fprintln(os.Stderr, "\nExamples:")
		fmt.Fprintf(os.Stderr, "  %s -db SafeInCloud_2017-03-19.xml -p \"Credit Cards,Banking,Insurance\" -logtostderr -v 5\n", script)
		fmt.Fprintf(os.Stderr, "  %s -db SafeInCloud_2017-03-19.xml -d \"Untagged\" -p \"Credit Cards,Banking,Insurance\"\n", script)
		fmt.Fprintf(os.Stderr, "  %s -db SafeInCloud_2017-03-19.xml -d \"Imported (SafeInCloud)\" -logtostderr -v 5\n", script)
		fmt.Fprintf(os.Stderr, "  %s -db SafeInCloud_2017-03-19.xml -p \"Accounting,Software,Inventor\" -logtostderr -v 3\n", script)
		fmt.Fprintln(os.Stderr, "\nAvailable flags:")
		flag.PrintDefaults()
	}

	flag.StringVar(&dbFile, "db", "", "An Exported SafeInCloud.xml path and filename.")
	flag.StringVar(&defaultFolder, "f", "Imported", "Default folder of unlabelled cards.")
	flag.StringVar(&priorityFoldersRaw, "p", "", "Priority folder of labels to assign in order (comma delimited).")
}
