/*Package main is an executable to import SafeInCloud into LastPass.

A simple utility to take an export from SafeInCloud and convert the cards
to LastPass sites and secure notes.

This is an opinionated tool as there are a number of assumptions made to how
the cards are organized, labelled and filled out.

Features

Finally, a SafeInCloud conversion tool that works - including attachment decoding.

* Converts SafeInCloud to LastPass CSV format
* Creates LastPass Sites if all required fields are present: title, website, username, password.
* Creates LastPass Secure Notes if not all of Sites required fields are found.
* Creates LastPass specialized Secure Notes if certain labels were used (Credit Cards, Banking, Insurance, etc). See below.
* Creates multiple LastPass Sites if multiple logins are specified on a single card.
* Extracts all file and image attachments.  LastPass CSV imports do not support file imports.  Will have to import manually.
* Flattens SafeInClouds' Labels, with logic, to LastPass' Folder structure.
* Ability to override/select/prioritize what Folder you want the cards imported into.

And more features.  The source code, specifically the main.go file, has a lot more
comments and details.

Installation

You can download a pre-compiled binary from the releases:

https://github.com/eduncan911/sic2lp/releases

Or, you can install from source:

    go install github.com/eduncan911/sic2lp

How to Use

Use the binary at a command prompt to execute.

    $ sic2lp -h
    Usage of sic2lp:
      sic2lp -db /path/to/SafeInCloud_Export.xml [options]

    Examples:
      sic2lp -db SafeInCloud_2017-03-19.xml -p "Credit Cards,Banking,Insurance" -logtostderr -v 5
      sic2lp -db SafeInCloud_2017-03-19.xml -d "Untagged" -p "Credit Cards,Banking,Insurance"
      sic2lp -db SafeInCloud_2017-03-19.xml -d "Imported (SafeInCloud)" -logtostderr -v 5
      sic2lp -db SafeInCloud_2017-03-19.xml -p "Accounting,Software,Inventor" -logtostderr -v 3

    Available flags:
      -db string
            An Exported SafeInCloud.xml path and filename.
      -f string
            Default folder of unlabelled cards. (default "Imported")
      -p string
            Priority folder of labels to assign in order (comma delimited).

    Logging Options:
      -logtostderr
            log to standard error instead of files
      -v value
            log level for V logs

See below for tips on how to prepare your SafeInCloud for the best possible import.

Preparation

Below is a list of recommendations to prepare your SafeInCloud database for the
best possible import.

* Sites

Note that all SafeInCloud cards are 'tested' to see if they are a "Site", and if so
are treated that way at LastPass.  This means auto-login, form-fills, etc.  In order
for Sites to be used, all of the following are required for each specific card you
want to login with:

    Card's Title (will use the card's Website if blank)
    Login (must be of type "login")
    Password (must be of type "password")
    Website (must be of type "website")

As long as the SafeInCloud field names and types match above, it will designated
as a Site for auto-login at LastPass.

Otherwise, the card will be created as a SecureNote (see below).

* Card Labels

Card Labels are used for two things: What folder to import into, and if the card
is to be treated as a SecureNote what NoteType to use.

Set your SafeInCloud card labels ahead of time so that this tool can import them into the proper
Folder at LastPass, as well as the proper SecureNote NoteType if it is not a site.

* SecureNotes

Below are the current labels this tool recognizes and
what SecureNote NoteType it will use.

    SafeInCloud Label -> LastPass SecureNote Type
    -----------------    ------------------------
    "Credit Cards"    -> "NoteType:Credit Card"
    "Banking"         -> "NoteType:Bank Account"
    "Databases"       -> "NoteType:Database"
    "Licenses"        -> "NoteType:Driver's License"
    "Insurance"       -> "NoteType:Insurance"
    "Membership"      -> "NoteType:Membership"
    "Passport"        -> "NoteType:Passport"
    "Servers"         -> "NoteType:Server"
    "Software"        -> "NoteType:Software License"

There are also more Secure Note types and they can be added by customizing the code.
Or, just open an issue and I'll try to add it for you.

To reap the full benefits of these matches, a more indepth update would be
to go into each Card and change their Field names to what LastPass expects.  See
below for "Card Fields."

* Card Fields

LastPass does not have the concept of "Field Names" or custom name/value items that
we can add at SafeInCloud.  Instead, LastPass SecureNotes uses plain text entries
prefixed with specific names for certain SecureNote types.

Start by downloading a list of LastPass's SecureNote Types (this is not all of them!):

https://helpdesk.lastpass.com/wp-content/uploads/Import_format_Secure_Note1.zip

For a complete list, log into your LastPass account and review the Secure Note types.

For example, the Bank_Account example uses the format of:

    NoteType:Bank Account
    Bank Name:
    Account Type:
    Routing Number:
    Account Number:

This tool handles the first one, NoteType:Bank Account, for you.  But the other
fields are clear text.

During importing, we have the opportunity to fill these out properly so that our
SafeInCloud data does not end up in a blob in the Extra section of all notes.

To do this, we have to change each card's fields to match the expected Field Name.

For example, in my SafeInCloud I created a Banking label and had the following
addition fields: Account #, Routing, Checking #, Saving #, etc.  To convert these
to LastPass, I had to rename these fields:

    "Account #"  -> "Login"
    "Routing"    -> "Routing Number"
    "Checking #" -> "Account Number"
    "Saving #    -> "Savings Number"

Even though only two of these four fields would match, the Extras section at LastPass
will neatly show the other two in a common format.

Note: SafeInCloud's "Template" feature is only good for creating new cards, not for
renaming fields of existing cards.  I know, that would have been much easier if it
did follow a relational model.

Customization

You can modify the behavior by editing the source code and running the tool
on your location machine.  All source is located in a single file to make it
easy for newcomers (not my typical code arrangement; but, it is easy to
follow).

1 - Download and install GoLang: https://golang.org/dl/

2 - Checkout the sourcecode with GoLang:

    go get github.com/eduncan911/sic2lp.git

3 - Change directory and open the main.go script with your favorite editor:

    cd $HOME/go/src/github.com/eduncan911/sic2lp
    open main.go

    cd %USERPROFILE%\go\src\github.com\eduncan911\sic2lp
    notepad main.go

4 - Modify the source as needed.

5 - Run the code with your changes:

    go run main.go -db <SafeInCloud_Export.xml> -p "Label1,Label2" -logtostderr -v 5

This is a verbose output command to help with debugging.

Release Notes

1.0.0
 - Initial release.

*/
package main
