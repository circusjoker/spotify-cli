package main

import (
	"fmt"

	tui "github.com/marcusolsson/tui-go"
	"github.com/zmb3/spotify"
)

// SideBar represents box with album list inside this box.
type SideBar struct {
	albumList *AlbumList
	box       *tui.Box
}

type Renderer interface {
	render() error
}

type PageRenderer interface {
	renderPage([]albumDescription, int, int) error
}

type DataFetcher interface {
	fetchUserAlbums() ([]albumDescription, error)
}

// AlbumList represents list of albums with underlying data,
// table to display, box in which table is places, indexes
// pointing to currently playing item, and last chosen items.
type AlbumList struct {
	client             SpotifyClient
	albumsDescriptions []albumDescription
	currDataIdx        int
	lastTwoSelected    []int
	table              *tui.Table
	box                *tui.Box

	Renderer
	PageRenderer
	DataFetcher
}

type albumDescription struct {
	artist string
	title  string
	uri    spotify.URI
}

var (
	visibleAlbums        = 45
	spotifyAPIPageSize   = 25
	spotifyAPIPageOffset = 25
	uiColumnWidth        = 20
)

// NewSideBar creates struct which holds references to
// SideBar Box and AlbumList placed inside SideBar
func NewSideBar(client SpotifyClient) (*SideBar, error) {
	al := newEmptyAlbumList(client)
	err := al.render()
	if err != nil {
		return nil, err
	}
	box := tui.NewHBox(al.box, tui.NewSpacer())
	return &SideBar{albumList: al, box: box}, nil
}

func newEmptyAlbumList(client SpotifyClient) *AlbumList {
	table := tui.NewTable(0, 0)
	table.SetColumnStretch(0, 1)
	table.SetColumnStretch(1, 1)
	table.SetColumnStretch(2, 4)

	albumListBox := tui.NewVBox(table, tui.NewSpacer())
	albumListBox.SetBorder(true)
	albumListBox.SetTitle("User albums")
	albumListBox.SetSizePolicy(tui.Preferred, tui.Expanding)

	return &AlbumList{
		client:             client,
		currDataIdx:        0,
		lastTwoSelected:    []int{-1, -1},
		table:              table,
		box:                albumListBox,
		albumsDescriptions: []albumDescription{},

		DataFetcher:  &FetchUserAlbumsStruct{client: client},
		PageRenderer: &RenderPageStruct{table: table},
	}
}

func (albumList *AlbumList) render() error {
	albumsDescriptions, err := albumList.DataFetcher.fetchUserAlbums()
	if err != nil {
		return err
	}
	albumList.albumsDescriptions = albumsDescriptions
	err = albumList.PageRenderer.renderPage(albumList.albumsDescriptions, 0, visibleAlbums)
	if err != nil {
		return err
	}
	albumList.table.OnSelectionChanged(albumList.onSelectedChanged())
	albumList.table.OnItemActivated(albumList.onItemActivaed())
	return nil
}

type FetchUserAlbumsStruct struct {
	client SpotifyClient
}

func (fetchUserAlbumsStruct *FetchUserAlbumsStruct) fetchUserAlbums() ([]albumDescription, error) {
	initialPage, err := fetchUserAlbumsStruct.client.CurrentUsersAlbumsOpt(&spotify.Options{Limit: &spotifyAPIPageSize})
	if err != nil {
		return nil, fmt.Errorf("could not fetch current user albums: %v", err)
	}
	userAlbums := make([]spotify.SavedAlbum, 0)
	userAlbums = append(userAlbums, initialPage.Albums...)

	page := initialPage
	for spotifyAPIPageOffset < initialPage.Total {
		page, err = fetchUserAlbumsStruct.client.CurrentUsersAlbumsOpt(&spotify.Options{
			Limit:  &initialPage.Limit,
			Offset: &spotifyAPIPageOffset,
		})
		if err != nil {
			return nil, fmt.Errorf("could not fetch page current user albums: %v", err)
		}
		spotifyAPIPageOffset += spotifyAPIPageSize
		userAlbums = append(userAlbums, page.Albums...)
	}

	albumsDescriptions := make([]albumDescription, 0)
	for _, album := range userAlbums {
		albumsDescriptions = append(albumsDescriptions, albumDescription{album.Name, album.Artists[0].Name, album.URI})
	}
	return albumsDescriptions, nil
}

func (albumList *AlbumList) onSelectedChanged() func(*tui.Table) {
	return func(t *tui.Table) {
		if albumList.nextPage() {
			err := albumList.renderPage(
				albumList.albumsDescriptions,
				(albumList.currDataIdx/visibleAlbums)*visibleAlbums,
				(albumList.currDataIdx/visibleAlbums)*visibleAlbums+visibleAlbums,
			)
			if err != nil {
				panic(err)
			}
			albumList.lastTwoSelected = []int{-1, -1}
			t.Select(1)
			return
		}
		if albumList.previousPage() {
			err := albumList.renderPage(
				albumList.albumsDescriptions,
				(albumList.currDataIdx/visibleAlbums)*visibleAlbums-visibleAlbums,
				(albumList.currDataIdx/visibleAlbums)*visibleAlbums,
			)
			if err != nil {
				panic(err)
			}
			albumList.lastTwoSelected = []int{visibleAlbums + 2, visibleAlbums + 1}
			t.Select(visibleAlbums)
			return
		}
		albumList.updateIndexes()
	}
}

func (albumList *AlbumList) nextPage() bool {
	return albumList.lastTwoSelected[0] == visibleAlbums-1 && albumList.lastTwoSelected[1] == visibleAlbums
}

func (albumList *AlbumList) previousPage() bool {
	return albumList.lastTwoSelected[1] == 1 && albumList.table.Selected() == 0 && albumList.currDataIdx >= visibleAlbums
}

func (albumList *AlbumList) updateIndexes() {
	albumList.lastTwoSelected[0] = albumList.lastTwoSelected[1]
	albumList.lastTwoSelected[1] = albumList.table.Selected()

	if albumList.lastTwoSelected[0] > albumList.lastTwoSelected[1] {
		albumList.currDataIdx--
	}
	if albumList.lastTwoSelected[0] < albumList.lastTwoSelected[1] {
		albumList.currDataIdx++
	}
}

func (albumList *AlbumList) onItemActivaed() func(*tui.Table) {
	return func(t *tui.Table) {
		albumList.client.PlayOpt(&spotify.PlayOptions{PlaybackContext: &albumList.albumsDescriptions[albumList.currDataIdx-2].uri})
	}
}

type RenderPageStruct struct {
	table *tui.Table
}

func (renderPageStruct *RenderPageStruct) renderPage(albumsDescriptions []albumDescription, start, end int) error {
	renderPageStruct.table.RemoveRows()
	renderPageStruct.table.AppendRow(
		tui.NewLabel("Title"),
		tui.NewLabel("Artist"),
	)
	if len(albumsDescriptions) == 0 {
		return fmt.Errorf("could not iterate over empty slice")
	}
	if len(albumsDescriptions) < end {
		end = len(albumsDescriptions) // This means that there is less user albums than there is displayed at once on the page.
	}
	for _, album := range albumsDescriptions[start:end] {
		renderPageStruct.table.AppendRow(
			tui.NewLabel(trimWithCommasIfTooLong(album.artist, uiColumnWidth)),
			tui.NewLabel(trimWithCommasIfTooLong(album.title, uiColumnWidth)),
		)
	}
	return nil
}

func trimWithCommasIfTooLong(text string, maxLength int) string {
	if len(text) > maxLength {
		text = text[:maxLength] + "..."
	}
	return text
}
