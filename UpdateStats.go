package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	// Selenium
	"github.com/tebeka/selenium"
	"github.com/tebeka/selenium/chrome"

	// Google Sheets
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/sheets/v4"

	// Excel Functions
	"github.com/360EntSecGroup-Skylar/excelize"
)

var screenshotDir = "screenshots"

type UpdateState struct {
	// Update State
	FirstBlockStart, SecondBlockStart, ThirdBlockStart int64
}

type Account struct {
	Platform    string
	Followers   int
	Likes       int
	AccountName string
	Difference  string
	SheetRowNum int
	CountNum    int
	FullURL     string
}

type Converter struct {
	Divider int64
	Suffix  string
}

func captureData(account *Account, driver selenium.WebDriver, screenshotPath string) {
	url := account.FullURL
	accountName := account.AccountName

	fmt.Println("URL " + url)

	if err := driver.Get(url); err != nil {
		fmt.Println(err)
		log.Fatal(err)
	}

	// Save screenshot
	pngBytes, err := driver.Screenshot()

	img, _, _ := image.Decode(bytes.NewReader(pngBytes))
	fileName := accountName + ".png"
	fullPath := filepath.Join(screenshotPath, fileName)
	outFile, err := os.Create(fullPath)
	defer outFile.Close()
	err = png.Encode(outFile, img)

	driver.SetImplicitWaitTimeout(time.Second * 30)

	followersXpath := "/html/body/div[1]/div/div[2]/div/div[1]/div/header/h2[1]/div[2]/strong"
	likesXpath := "/html/body/div[1]/div/div[2]/div/div[1]/div/header/h2[1]/div[3]/strong"

	followers, err := driver.FindElement(selenium.ByXPATH, followersXpath)
	errChk(err)

	likes, err := driver.FindElement(selenium.ByXPATH, likesXpath)
	errChk(err)

	followerCount, err := followers.Text()
	likeCount, err := likes.Text()
	followerNumber := convertCountToNumber(followerCount)
	likesNumber := convertCountToNumber(likeCount)

	account.Followers = followerNumber
	account.Likes = likesNumber
	fmt.Printf("Account at the end %v\n", account)
}

func errChk(err error) {
	if err != nil {
		fmt.Println(err)
		log.Fatal(err)
	}
}

func convertCountToNumber(count string) int {
	switch unit := count[len(count)-1:]; unit {
	case "M":
		period := strings.Index(count, ".")
		beforePeriodDigits := ""
		afterPeriodDigits := ""
		if period == -1 {
			beforePeriodDigits = count[:strings.Index(count, "M")]
			afterPeriodDigits = ""
		} else {
			beforePeriodDigits = count[:period]
			afterPeriodDigits = count[period+1 : len(count)-1]
		}
		numberOfDigitsAfter := len(afterPeriodDigits)
		zeroes := strings.Repeat("0", 6-numberOfDigitsAfter)
		newNumber := fmt.Sprintf("%s%s%s", beforePeriodDigits, afterPeriodDigits, zeroes)
		returnNumber, _ := strconv.Atoi(newNumber)
		return returnNumber
	case "K":
		number := count[:strings.Index(count, "K")]
		period := strings.Index(count, ".")
		var numberBefore, numberAfter, combined string
		if period == -1 {
			period = 1
			numberBefore = number
			numberAfter = ""
		} else {
			numberBefore = count[:period]
			numberAfter = count[period+1 : strings.Index(count, "K")]
		}
		if numberAfter == "" {
			combined = numberBefore
		} else {
			combined = fmt.Sprintf("%s%s", numberBefore, numberAfter)
		}
		zeroes := strings.Repeat("0", 3-len(numberAfter))
		newNumber := combined + zeroes
		returnNumber, _ := strconv.Atoi(newNumber)
		return returnNumber
	default:
		returnNumber, _ := strconv.Atoi(count)
		return returnNumber
	}
}

// Save state from File
func loadSaveStateFromFile(file string) (*UpdateState, error) {
	var state UpdateState
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	err = json.NewDecoder(f).Decode(&state)
	return &state, err
}

func saveSaveStateToFile(path string, saveState *UpdateState) {
	fmt.Printf("Saving saveState file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(saveState)
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

// Retrieve a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config) *http.Client {
	// The file token.json stores the user's access and refresh tokens, and is
	// created automatically when the authorization flow completes for the first
	// time.
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code: %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}
	return tok
}

func duplicateSheet(srv *sheets.Service, spreadSheetID string, newSheetName string, sheetID int64, insertIndex int64) (newSheetID sheets.SheetProperties) {
	duplicateSheetRequest := sheets.DuplicateSheetRequest{
		NewSheetName:     newSheetName,
		SourceSheetId:    sheetID,
		InsertSheetIndex: insertIndex,
	}

	dupSheetRequest := sheets.Request{
		DuplicateSheet: &duplicateSheetRequest,
	}
	requests := []*sheets.Request{}
	requests = append(requests, &dupSheetRequest)

	batchReq := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: requests,
	}

	resp, err := srv.Spreadsheets.BatchUpdate(spreadSheetID, batchReq).Do()
	if err != nil {
		log.Fatal(err)
	}
	newSheetID = *resp.Replies[0].DuplicateSheet.Properties
	return newSheetID
}

func differenceFormat(difference string) (formattedString string) {
	numberOfZeroes := 0
	newString := difference
	for true {
		innerString := strings.TrimSuffix(newString, "0")
		if newString == innerString {
			break
		} else {
			numberOfZeroes++
			newString = innerString
		}
	}
	switch fullLength := len(difference); {
	case fullLength >= 10:
		fmt.Println("Billion")
	case fullLength >= 7:
		if len(newString) <= 1 {
			formattedString = newString + "M"
		} else {
			formattedString = newString[0:1] + "." + newString[1:] + "M"
		}
		fmt.Println(formattedString)
	case fullLength >= 4:
		if len(newString) <= 1 {
			formattedString = newString + "00K"
		} else {
			switch thousandLength := len(newString); {
			case thousandLength == 4 && numberOfZeroes == 1:
				formattedString = newString[0:2] + "." + newString[2:] + "K"
			case thousandLength == 4 && numberOfZeroes == 2:
				formattedString = newString[0:3] + "." + newString[3:] + "K"
			case thousandLength < 4:
				formattedString = difference
			}
		}
	}
	return formattedString
}

func spreadSheetWork(srv *sheets.Service, newSheetName string, oldSheetName string, dateFormat string, state *UpdateState, accounts []*Account) {
	//spreadSheetID := "1GRXYwIcmA2fQbOqr4ihO_4MY9Ok80-Su7ib2B7YV1lo"
	spreadSheetID := "1ddu6XIRM7tajfJM0jdPVs0xvy2oKwe_M2Y9g5vcask4"
	var sheetID int64 = 2031708574
	conversionMatrix := []Converter{}
	conversionMatrix = append(conversionMatrix, Converter{Divider: 1e3, Suffix: "K"})
	conversionMatrix = append(conversionMatrix, Converter{Divider: 1e6, Suffix: "M"})
	conversionMatrix = append(conversionMatrix, Converter{Divider: 1e9, Suffix: "G"})
	conversionMatrix = append(conversionMatrix, Converter{Divider: 1e12, Suffix: "T"})
	conversionMatrix = append(conversionMatrix, Converter{Divider: 1e15, Suffix: "P"})
	conversionMatrix = append(conversionMatrix, Converter{Divider: 1e18, Suffix: "E"})

	// Let's do some math on where things are going to go.
	numberOfAccounts := int64(len(accounts))
	upperHeaderRowNumber := int64(2)
	endOfUpperSection := upperHeaderRowNumber + numberOfAccounts
	lowerHeaderRowNumber := endOfUpperSection + 2 // From header row, go down one, then down number of accounts, then down 2 more

	requests := []*sheets.Request{}

	spreadSheetsCall, err := srv.Spreadsheets.Get(spreadSheetID).Do()
	if err != nil {
		log.Fatal(err)
	}
	spreadSheets := spreadSheetsCall.Sheets

	var oldSheetIndex int64

	// Get the index and the sheetID from the last run
	for _, sh := range spreadSheets {
		if sh.Properties.Title == oldSheetName {
			oldSheetIndex = sh.Properties.Index + 1
			sheetID = sh.Properties.SheetId
		}
	}

	// First duplicate sheet
	newSheet := duplicateSheet(srv, spreadSheetID, newSheetName, sheetID, oldSheetIndex)

	// Insert new first section column
	firstSectionColumnInsert := sheets.DimensionRange{
		SheetId:    newSheet.SheetId,
		Dimension:  "COLUMNS",
		StartIndex: state.FirstBlockStart, // X
		EndIndex:   state.FirstBlockStart + 1,
	}

	columnToHide := sheets.DimensionRange{
		SheetId:    newSheet.SheetId,
		Dimension:  "COLUMNS",
		StartIndex: state.FirstBlockStart - 1,
		EndIndex:   state.FirstBlockStart,
	}

	hideRequest := sheets.UpdateDimensionPropertiesRequest{
		Range: &columnToHide,
		Properties: &sheets.DimensionProperties{
			HiddenByUser: true,
		},
		Fields: "*",
	}

	insertFirstSectionColumnRequest := sheets.InsertDimensionRequest{
		InheritFromBefore: true,
		Range:             &firstSectionColumnInsert,
	}

	insertColumnFirstSectionRequest := sheets.Request{
		InsertDimension: &insertFirstSectionColumnRequest,
	}
	hideColumnRequest := sheets.Request{
		UpdateDimensionProperties: &hideRequest,
	}
	requests = []*sheets.Request{}
	requests = append(requests, &insertColumnFirstSectionRequest)
	requests = append(requests, &hideColumnRequest)

	batchReq := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: requests,
	}

	_, err = srv.Spreadsheets.BatchUpdate(spreadSheetID, batchReq).Do()

	if err != nil {
		log.Fatal(err)
	}

	// Update Header Values
	// Followers
	var values sheets.ValueRange
	value := []interface{}{dateFormat}
	values.Values = append(values.Values, value)
	values.MajorDimension = "ROWS"

	newColumn := state.FirstBlockStart + 1
	newColumnName, err := excelize.ColumnNumberToName(int(newColumn))
	fmt.Println("new column is " + newColumnName)
	fmt.Println("dateFormat is " + dateFormat)

	//followersUpdateCell := newSheet.Title + "!" + newColumnName + "2:" + newColumnName + "2"
	followersUpdateCell := fmt.Sprintf("%s!%s%d:%s%d", newSheet.Title, newColumnName, upperHeaderRowNumber, newColumnName, upperHeaderRowNumber)
	fmt.Println(followersUpdateCell)
	values.Range = followersUpdateCell

	updateCall := srv.Spreadsheets.Values.Update(spreadSheetID, followersUpdateCell, &values)
	updateCall.ValueInputOption("USER_ENTERED")

	_, err = updateCall.Do()

	// Likes
	// likesUpdateCell := newSheet.Title + "!" + newColumnName + "28:" + newColumnName + "28"
	//likesUpdateCell := newSheet.Title + "!" + newColumnName + "29:" + newColumnName + "29"
	likesUpdateCell := fmt.Sprintf("%s!%s%d:%s%d", newSheet.Title, newColumnName, lowerHeaderRowNumber, newColumnName, lowerHeaderRowNumber)
	fmt.Println(likesUpdateCell)
	values.Range = likesUpdateCell

	updateCall = srv.Spreadsheets.Values.Update(spreadSheetID, likesUpdateCell, &values)
	updateCall.ValueInputOption("USER_ENTERED")

	_, err = updateCall.Do()

	secondSectionCopyPasteRequestTop := sheets.Request{
		CopyPaste: &sheets.CopyPasteRequest{
			PasteOrientation: "NORMAL",
			PasteType:        "PASTE_VALUES",
			Source: &sheets.GridRange{
				SheetId:          newSheet.SheetId,
				StartColumnIndex: state.SecondBlockStart - 1,
				StartRowIndex:    upperHeaderRowNumber - 1, //1
				EndColumnIndex:   state.SecondBlockStart,
				EndRowIndex:      endOfUpperSection, //26
			},
			Destination: &sheets.GridRange{
				SheetId:          newSheet.SheetId,
				StartColumnIndex: state.SecondBlockStart - 2,
				StartRowIndex:    upperHeaderRowNumber - 1,
				EndColumnIndex:   state.SecondBlockStart - 1,
				EndRowIndex:      endOfUpperSection,
			},
		},
	}

	secondSectionCopyPasteRequestBottom := sheets.Request{
		CopyPaste: &sheets.CopyPasteRequest{
			PasteOrientation: "NORMAL",
			PasteType:        "PASTE_VALUES",
			Source: &sheets.GridRange{
				SheetId:          newSheet.SheetId,
				StartColumnIndex: state.SecondBlockStart - 1,
				StartRowIndex:    lowerHeaderRowNumber - 1, //28
				EndColumnIndex:   state.SecondBlockStart,
				EndRowIndex:      lowerHeaderRowNumber + numberOfAccounts, //53
			},
			Destination: &sheets.GridRange{
				SheetId:          newSheet.SheetId,
				StartColumnIndex: state.SecondBlockStart - 2,
				StartRowIndex:    lowerHeaderRowNumber - 1, //28
				EndColumnIndex:   state.SecondBlockStart - 1,
				EndRowIndex:      lowerHeaderRowNumber + numberOfAccounts, //53
			},
		},
	}

	// thirdSectionCopyPasteRequestTop := sheets.Request{
	// 	CopyPaste: &sheets.CopyPasteRequest{
	// 		PasteOrientation: "NORMAL",
	// 		PasteType:        "PASTE_VALUES",
	// 		Source: &sheets.GridRange{
	// 			SheetId:          newSheet.SheetId,
	// 			StartColumnIndex: state.ThirdBlockStart - 1,
	// 			StartRowIndex:    1,
	// 			EndColumnIndex:   state.ThirdBlockStart,
	// 			EndRowIndex:      26,
	// 		},
	// 		Destination: &sheets.GridRange{
	// 			SheetId:          newSheet.SheetId,
	// 			StartColumnIndex: state.ThirdBlockStart - 2,
	// 			StartRowIndex:    1,
	// 			EndColumnIndex:   state.ThirdBlockStart - 1,
	// 			EndRowIndex:      26,
	// 		},
	// 	},
	// }

	// thirdSectionCopyPasteRequestBottom := sheets.Request{
	// 	CopyPaste: &sheets.CopyPasteRequest{
	// 		PasteOrientation: "NORMAL",
	// 		PasteType:        "PASTE_VALUES",
	// 		Source: &sheets.GridRange{
	// 			SheetId:          newSheet.SheetId,
	// 			StartColumnIndex: state.ThirdBlockStart - 1,
	// 			StartRowIndex:    28,
	// 			EndColumnIndex:   state.ThirdBlockStart,
	// 			EndRowIndex:      53,
	// 		},
	// 		Destination: &sheets.GridRange{
	// 			SheetId:          newSheet.SheetId,
	// 			StartColumnIndex: state.ThirdBlockStart - 2,
	// 			StartRowIndex:    28,
	// 			EndColumnIndex:   state.ThirdBlockStart - 1,
	// 			EndRowIndex:      53,
	// 		},
	// 	},
	// }

	// Let's update values
	//followerReadRange := newSheet.Title + "!" + "B3:B26"
	followerReadRange := fmt.Sprintf("%s!B%d:B%d", newSheet.Title, upperHeaderRowNumber+1, upperHeaderRowNumber+numberOfAccounts)
	fmt.Printf("\n\tfollowerReadRange: %s\n", followerReadRange)
	followerReadResp, err := srv.Spreadsheets.Values.Get(spreadSheetID, followerReadRange).Do()
	if err != nil {
		log.Fatal(err)
	}
	if len(followerReadResp.Values) == 0 {
		fmt.Println("COULDN'T FIND FOLLOWERS")
	} else {
		for i, row := range followerReadResp.Values {
			//fmt.Printf("%s\n", row[0])
			platform := row[0]
			for j, accObj := range accounts {
				if accObj.Platform == platform {
					accObj.SheetRowNum = j
					var values sheets.ValueRange
					// Followers
					value := []interface{}{accObj.Followers}
					values.Values = append(values.Values, value)
					values.MajorDimension = "ROWS"

					followersUpdateCell := newSheet.Title + "!" + newColumnName + strconv.Itoa(i+3) + ":" + newColumnName + strconv.Itoa(i+3)

					values.Range = followersUpdateCell

					updateCall := srv.Spreadsheets.Values.Update(spreadSheetID, followersUpdateCell, &values)
					updateCall.ValueInputOption("USER_ENTERED")

					_, err = updateCall.Do()
				}
			}
		}
	}
	//likesReadRange := newSheet.Title + "!" + "B30:B53"
	likesReadRange := fmt.Sprintf("%s!B%d:B%d", newSheet.Title, lowerHeaderRowNumber+1, lowerHeaderRowNumber+numberOfAccounts)
	likesReadResp, err := srv.Spreadsheets.Values.Get(spreadSheetID, likesReadRange).Do()
	if len(likesReadResp.Values) == 0 {
		fmt.Println("COULDN'T FIND LIKES")
	} else {
		for i, row := range likesReadResp.Values {
			//fmt.Printf("%s\n", row[0])
			paltform := row[0]
			for _, accObj := range accounts {
				if accObj.Platform == paltform {
					var values sheets.ValueRange
					// Likes
					values = sheets.ValueRange{}
					value = []interface{}{accObj.Likes}
					values.Values = append(values.Values, value)
					values.MajorDimension = "ROWS"
					cellnumber := i + int(lowerHeaderRowNumber) + 1
					likesUpdateCell := newSheet.Title + "!" + newColumnName + strconv.Itoa(cellnumber) + ":" + newColumnName + strconv.Itoa(cellnumber)

					values.Range = likesUpdateCell

					updateCall = srv.Spreadsheets.Values.Update(spreadSheetID, likesUpdateCell, &values)
					updateCall.ValueInputOption("USER_ENTERED")

					_, err = updateCall.Do()
				}
			}
		}
	}

	mainCopyPasteRequestSecondTop := sheets.Request{
		CopyPaste: &sheets.CopyPasteRequest{
			PasteOrientation: "NORMAL",
			PasteType:        "PASTE_VALUES",
			Source: &sheets.GridRange{
				SheetId:          newSheet.SheetId,
				StartColumnIndex: state.FirstBlockStart,
				StartRowIndex:    upperHeaderRowNumber - 1, //1
				EndColumnIndex:   state.FirstBlockStart + 1,
				EndRowIndex:      endOfUpperSection, //26
			},
			Destination: &sheets.GridRange{
				SheetId:          newSheet.SheetId,
				StartColumnIndex: state.SecondBlockStart - 1,
				StartRowIndex:    upperHeaderRowNumber - 1, //1
				EndColumnIndex:   state.SecondBlockStart,
				EndRowIndex:      endOfUpperSection, //26
			},
		},
	}

	mainCopyPasteRequestSecondBottom := sheets.Request{
		CopyPaste: &sheets.CopyPasteRequest{
			PasteOrientation: "NORMAL",
			PasteType:        "PASTE_VALUES",
			Source: &sheets.GridRange{
				SheetId:          newSheet.SheetId,
				StartColumnIndex: state.FirstBlockStart,
				StartRowIndex:    lowerHeaderRowNumber - 1, //28
				EndColumnIndex:   state.FirstBlockStart + 1,
				EndRowIndex:      lowerHeaderRowNumber + numberOfAccounts, //53
			},
			Destination: &sheets.GridRange{
				SheetId:          newSheet.SheetId,
				StartColumnIndex: state.SecondBlockStart - 1,
				StartRowIndex:    lowerHeaderRowNumber - 1, //28
				EndColumnIndex:   state.SecondBlockStart,
				EndRowIndex:      lowerHeaderRowNumber + numberOfAccounts, //53
			},
		},
	}

	// mainCopyPasteRequestThirdTop := sheets.Request{
	// 	CopyPaste: &sheets.CopyPasteRequest{
	// 		PasteOrientation: "NORMAL",
	// 		PasteType:        "PASTE_VALUES",
	// 		Source: &sheets.GridRange{
	// 			SheetId:          newSheet.SheetId,
	// 			StartColumnIndex: state.FirstBlockStart,
	// 			StartRowIndex:    1,
	// 			EndColumnIndex:   state.FirstBlockStart + 1,
	// 			EndRowIndex:      26,
	// 		},
	// 		Destination: &sheets.GridRange{
	// 			SheetId:          newSheet.SheetId,
	// 			StartColumnIndex: state.ThirdBlockStart - 1,
	// 			StartRowIndex:    1,
	// 			EndColumnIndex:   state.ThirdBlockStart,
	// 			EndRowIndex:      26,
	// 		},
	// 	},
	// }

	// mainCopyPasteRequestThirdBottom := sheets.Request{
	// 	CopyPaste: &sheets.CopyPasteRequest{
	// 		PasteOrientation: "NORMAL",
	// 		PasteType:        "PASTE_VALUES",
	// 		Source: &sheets.GridRange{
	// 			SheetId:          newSheet.SheetId,
	// 			StartColumnIndex: state.FirstBlockStart,
	// 			StartRowIndex:    28,
	// 			EndColumnIndex:   state.FirstBlockStart + 1,
	// 			EndRowIndex:      53,
	// 		},
	// 		Destination: &sheets.GridRange{
	// 			SheetId:          newSheet.SheetId,
	// 			StartColumnIndex: state.ThirdBlockStart - 1,
	// 			StartRowIndex:    28,
	// 			EndColumnIndex:   state.ThirdBlockStart,
	// 			EndRowIndex:      53,
	// 		},
	// 	},
	// }

	sortSpecArray := []*sheets.SortSpec{}
	sortSpecArray = append(sortSpecArray, &sheets.SortSpec{SortOrder: "DESCENDING", DimensionIndex: newColumn - 1})

	sortMainSectionFollowersRequest := sheets.Request{
		SortRange: &sheets.SortRangeRequest{
			Range: &sheets.GridRange{
				SheetId:          newSheet.SheetId,
				StartColumnIndex: 1,                    // B
				StartRowIndex:    upperHeaderRowNumber, //2 meaning 3
				EndColumnIndex:   newColumn,
				EndRowIndex:      endOfUpperSection,
			},
			SortSpecs: sortSpecArray,
		},
	}

	sortMainSectionLikesRequest := sheets.Request{
		SortRange: &sheets.SortRangeRequest{
			Range: &sheets.GridRange{
				SheetId:          newSheet.SheetId,
				StartColumnIndex: 1,                    // B
				StartRowIndex:    lowerHeaderRowNumber, // 29
				EndColumnIndex:   newColumn,
				EndRowIndex:      lowerHeaderRowNumber + numberOfAccounts, //53
			},
			SortSpecs: sortSpecArray,
		},
	}

	requests = []*sheets.Request{}
	requests = append(requests, &secondSectionCopyPasteRequestTop)
	requests = append(requests, &secondSectionCopyPasteRequestBottom)
	// requests = append(requests, &thirdSectionCopyPasteRequestTop)
	// requests = append(requests, &thirdSectionCopyPasteRequestBottom)
	requests = append(requests, &mainCopyPasteRequestSecondTop)
	requests = append(requests, &mainCopyPasteRequestSecondBottom)
	// requests = append(requests, &mainCopyPasteRequestThirdTop)
	// requests = append(requests, &mainCopyPasteRequestThirdBottom)
	requests = append(requests, &sortMainSectionFollowersRequest)
	requests = append(requests, &sortMainSectionLikesRequest)

	batchReq = &sheets.BatchUpdateSpreadsheetRequest{
		Requests: requests,
	}

	_, err = srv.Spreadsheets.BatchUpdate(spreadSheetID, batchReq).Do()
	if err != nil {
		log.Fatal(err)
	}

	// Now let's set the names for the differences
	followerDifferenceRange := newSheet.Title + "!" + "AM3:AM26"
	followerDifferenceRangeResp, err := srv.Spreadsheets.Values.Get(spreadSheetID, followerDifferenceRange).Do()
	if err != nil {
		log.Fatal(err)
	}
	if len(followerDifferenceRangeResp.Values) == 0 {
		fmt.Println("COULDN'T FIND PLATFORMS")
	} else {
		for i, row := range followerDifferenceRangeResp.Values {
			sheetString := row[0].(string)
			normalizedString := strings.Replace(sheetString, ",", "", -1)
			// difference, _ := strconv.ParseInt(normalizedString, 10, 64)
			differenceString := differenceFormat(normalizedString)
			for _, acctObj := range accounts {
				if acctObj.SheetRowNum == i {
					acctObj.Difference = acctObj.Platform + "(" + differenceString + ")"
					// fmt.Printf("Platform %s will have this new string %s\n", acctObj.Platform, acctObj.Difference)
				}
			}
		}
	}
}

func main() {
	fmt.Printf("Beginning the run...\n")
	// Create screenshot directory
	currentWD, err := os.Getwd()
	errChk(err)
	datePath := time.Now().Format("2006-01-02")
	screenshotPath := filepath.Join(currentWD, screenshotDir, datePath)
	if _, err := os.Stat(screenshotPath); os.IsNotExist(err) {
		os.MkdirAll(screenshotPath, os.ModePerm)
	}
	// Setup Selenium
	const (
		seleniumURL = "http://192.168.1.3:4444/wd/hub"
	)
	caps := selenium.Capabilities{
		"browserName": "chrome",
	}
	chromeOptions := chrome.Capabilities{
		Args: []string{
			//"--user-agent=Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/83.0.4103.116 Safari/537.36",
			"--user-agent=Applebot",
			"--lang=en_US",
			"--headless",
			"--window-size=1920,1080",
		},
		ExcludeSwitches: []string{
			"enable-automation",
		},
	}
	caps.AddChrome(chromeOptions)
	// Create the web driver
	driver, err := selenium.NewRemote(caps, seleniumURL)
	defer driver.Quit()
	errChk(err)

	// Sheets API setup
	state, err := loadSaveStateFromFile("saveState-testing.json")
	if err != nil {
		log.Fatal(err)
	}
	b, err := ioutil.ReadFile("credentials.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	// If modifying these scopes, delete your previously saved token.json.
	config, err := google.ConfigFromJSON(b, "https://www.googleapis.com/auth/spreadsheets", "https://www.googleapis.com/auth/drive", "https://www.googleapis.com/auth/drive.file")
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(config)

	srv, err := sheets.New(client)
	if err != nil {
		log.Fatalf("Unable to retrieve Sheets client: %v", err)
	}

	// Let's find how many accounts we're dealing with today
	urlSpreadsheetID := "1GRXYwIcmA2fQbOqr4ihO_4MY9Ok80-Su7ib2B7YV1lo"
	urlSheetName := "TikTok URLs!"
	urlRanges := urlSheetName + "A1:C256"
	batchGetCall := srv.Spreadsheets.Values.Get(urlSpreadsheetID, urlRanges)
	batchGetCall = batchGetCall.MajorDimension("COLUMNS")

	resp, err := batchGetCall.Do()

	if err != nil {
		log.Fatal(err)
	}

	lookingForSpace := false
	accounts := []*Account{}
	for rowNum, row := range resp.Values[0] {
		rowValue := fmt.Sprintf("%v", row)
		if rowValue == "1" {
			lookingForSpace = true
		}
		if lookingForSpace {
			fullURL := fmt.Sprintf("%s", resp.Values[2][rowNum])
			accountNumber, _ := strconv.Atoi(rowValue)
			newAccount := &Account{
				CountNum:    accountNumber,
				SheetRowNum: rowNum + 1,
				Platform:    fmt.Sprintf("%s", resp.Values[1][rowNum]),
				AccountName: fullURL[strings.Index(fullURL, "@")+1 : len(fullURL)],
				FullURL:     fullURL,
			}
			accounts = append(accounts, newAccount)
		}
		if lookingForSpace && rowValue == "" {
			break
		}
	}

	// Read in the URLs
	for _, account := range accounts {
		captureData(account, driver, screenshotPath)
	}

	fmt.Printf("\n\tFirst account: %v \n", accounts[0])

	// Time to go to work!
	currentDate := time.Now()
	dateFormat := currentDate.Format("01/02/2006")
	dayOfWeek := currentDate.Weekday()
	prevDate := time.Now().AddDate(0, 0, -7)
	// switch prevDate.Weekday().String() {
	// case "Monday":
	// 	prevDate = time.Now().AddDate(0, 0, -3)
	// case "Tuesday":
	// 	prevDate = time.Now().AddDate(0, 0, -1)
	// case "Friday":
	// 	prevDate = time.Now().AddDate(0, 0, -3)
	// }
	oldSheetName := prevDate.Format("01/02/2006") + " (" + prevDate.Weekday().String()[:1] + ")"
	//oldSheetName := "07/01/2020 (W)"
	fmt.Println("Old Sheet Name " + oldSheetName)
	newSheetName := dateFormat + " (" + dayOfWeek.String()[:1] + ")"
	oldSheetName = "08/04/2020 (T)"
	newSheetName = "TEST"
	spreadSheetWork(srv, newSheetName, oldSheetName, dateFormat, state, accounts)
	// state.FirstBlockStart++
	// state.SecondBlockStart++
	// state.ThirdBlockStart++
	// saveSaveStateToFile("saveState.json", state)
}
