// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package model

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path"
	"strings"
	"sync"
	"wiki-kernel/util"

	"github.com/88250/gulu"
	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav/caldav"
	"github.com/siyuan-note/logging"
)

const (
	// REF: https://developers.google.com/calendar/caldav/v2/guide
	CalDavPrefixPath          = "/caldav"
	CalDavPrincipalsPath      = CalDavPrefixPath + "/principals"       // 0 resourceTypeRoot
	CalDavUserPrincipalPath   = CalDavPrincipalsPath + "/main"         // 1 resourceTypeUserPrincipal
	CalDavHomeSetPath         = CalDavUserPrincipalPath + "/calendars" // 2 resourceTypeCalendarHomeSet
	CalDavDefaultCalendarPath = CalDavHomeSetPath + "/default"         // 3 resourceTypeCalendar

	CalDavDefaultCalendarName = "default"

	CalDavCalendarsMetaDataFilePath = CalDavHomeSetPath + "/calendars.json"

	ICalendarFileExt = "." + ical.Extension // .ics
)

type CalDavPathDepth int

const (
	calDavPathDepth_Root          CalDavPathDepth = 1 + iota // /caldav
	calDavPathDepth_Principals                               // /caldav/principals
	calDavPathDepth_UserPrincipal                            // /caldav/principals/main
	calDavPathDepth_HomeSet                                  // /caldav/principals/main/calendars
	calDavPathDepth_Calendar                                 // /caldav/principals/main/calendars/default
	calDavPathDepth_Object                                   // /caldav/principals/main/calendars/default/id.ics
)

var (
	calendarMaxResourceSize       int64 = 0
	calendarSupportedComponentSet       = []string{"VEVENT", "VTODO"}

	defaultCalendar = caldav.Calendar{
		Path:                  CalDavDefaultCalendarPath,
		Name:                  CalDavDefaultCalendarName,
		Description:           "Default calendar",
		MaxResourceSize:       calendarMaxResourceSize,
		SupportedComponentSet: calendarSupportedComponentSet,
	}

	ErrorCalDavPathInvalid = errors.New("CalDAV: path is invalid")

	ErrorCalDavCalendarNotFound    = errors.New("CalDAV: calendar not found")
	ErrorCalDavCalendarPathInvalid = errors.New("CalDAV: calendar path is invalid")

	ErrorCalDavCalendarObjectNotFound    = errors.New("CalDAV: calendar object not found")
	ErrorCalDavCalendarObjectPathInvalid = errors.New("CalDAV: calendar object path is invalid")
)

// CalendarsMetaDataFilePath returns the absolute path of the calendars' meta data file
func CalendarsMetaDataFilePath() string {
	return DavPath2DirectoryPath(CalDavCalendarsMetaDataFilePath)
}

func GetCalDavPathDepth(urlPath string) CalDavPathDepth {
	urlPath = PathCleanWithSlash(urlPath)
	return CalDavPathDepth(len(strings.Split(urlPath, "/")) - 1)
}

// GetCardDavPathDepth parses
func ParseCalendarObjectPath(objectPath string) (calendarPath string, objectID string, err error) {
	calendarPath, objectFileName := path.Split(objectPath)
	calendarPath = PathCleanWithSlash(calendarPath)
	objectID = path.Base(objectFileName)
	objectFileExt := util.Ext(objectFileName)

	if GetCalDavPathDepth(calendarPath) != calDavPathDepth_Calendar {
		err = ErrorCalDavCalendarPathInvalid
		return
	}

	if objectFileExt != ICalendarFileExt {
		err = ErrorCalDavCalendarObjectPathInvalid
		return
	}

	return
}

// LoadCalendarObject loads a iCalendar file (*.ics)
func LoadCalendarObject(filePath string) (calendar *ical.Calendar, err error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		logging.LogErrorf("read iCalendar file [%s] failed: %s", filePath, err)
		return
	}

	decoder := ical.NewDecoder(bytes.NewReader(data))
	calendar, err = decoder.Decode()
	return
}

type Calendars struct {
	dataRoot          string
	loaded            bool
	changed           bool
	lock              sync.Mutex // load & save
	calendars         sync.Map   // Path -> *Calendar
	calendarsMetaData []*caldav.Calendar
}

func defaultCalendarMetaData() *caldav.Calendar {
	ret := defaultCalendar
	ret.SupportedComponentSet = append([]string(nil), defaultCalendar.SupportedComponentSet...)
	return &ret
}

func (c *Calendars) directoryPath(davPath string) (string, error) {
	if c == nil || c.dataRoot == "" {
		return "", errDAVOwnerUnbound
	}
	return davStoragePath(c.dataRoot, davPath)
}

func (c *Calendars) metaDataFilePath() (string, error) {
	return c.directoryPath(CalDavCalendarsMetaDataFilePath)
}

func (c *Calendars) load() error {
	return c.loadContext(context.Background())
}

func (c *Calendars) loadContext(ctx context.Context) error {
	if ctx == nil {
		return errDAVContextUnbound
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	c.calendars.Clear()

	// load calendars meta data file
	calendarsMetaDataFilePath, err := c.metaDataFilePath()
	if err != nil {
		return err
	}
	metaData, err := os.ReadFile(calendarsMetaDataFilePath)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if os.IsNotExist(err) {
		// create & save default calendar
		c.calendarsMetaData = []*caldav.Calendar{defaultCalendarMetaData()}
		if err := c.saveCalendarsMetaData(); err != nil {
			return err
		}
	} else {
		// load meta data file
		c.calendarsMetaData = []*caldav.Calendar{}
		if err = gulu.JSON.UnmarshalJSON(metaData, &c.calendarsMetaData); err != nil {
			logging.LogErrorf("unmarshal address books meta data failed: %s", err)
			return err
		}
	}

	// load iCalendar files (*.ics)
	tasks := make([]func(context.Context) error, 0, len(c.calendarsMetaData))
	for _, calendarMetaData := range c.calendarsMetaData {
		calendarMetaData := calendarMetaData
		directoryPath, err := c.directoryPath(calendarMetaData.Path)
		if err != nil {
			return err
		}
		calendar := &Calendar{
			Changed:       false,
			DirectoryPath: directoryPath,
			MetaData:      calendarMetaData,
			Objects:       sync.Map{},
		}
		c.calendars.Store(calendarMetaData.Path, calendar)
		tasks = append(tasks, func(life context.Context) error { return calendar.loadContext(life) })
	}
	if err := runDAVTasks(ctx, tasks); err != nil {
		return err
	}

	c.loaded = true
	c.changed = false
	return nil
}

// save all calendars
func (c *Calendars) save(force bool) error {
	return c.saveContext(context.Background(), force)
}

func (c *Calendars) saveContext(ctx context.Context, force bool) error {
	if ctx == nil {
		return errDAVContextUnbound
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if force || c.changed {
		// save calendars meta data
		if err := c.saveCalendarsMetaData(); err != nil {
			return err
		}

		// save all calendar object to *.ics files
		var tasks []func(context.Context) error
		c.calendars.Range(func(path any, value any) bool {
			calendar := value.(*Calendar)
			tasks = append(tasks, func(life context.Context) error { return calendar.saveContext(life, force) })
			return true
		})
		if err := runDAVTasks(ctx, tasks); err != nil {
			return err
		}
		c.changed = false
	}
	return nil
}

// save all calendars meta data
func (c *Calendars) saveCalendarsMetaData() error {
	metaDataFilePath, err := c.metaDataFilePath()
	if err != nil {
		return err
	}
	return SaveMetaData(c.calendarsMetaData, metaDataFilePath)
}

func (c *Calendars) Load() error {
	return c.LoadContext(context.Background())
}

func (c *Calendars) LoadContext(ctx context.Context) error {
	if ctx == nil {
		return errDAVContextUnbound
	}
	c.lock.Lock()
	defer c.lock.Unlock()

	if !c.loaded {
		return c.loadContext(ctx)
	}
	return ctx.Err()
}

func (c *Calendars) GetObject(objectPath string) (calendar *Calendar, calendarObject *CalendarObject, err error) {
	calendarPath, objectID, err := ParseCalendarObjectPath(objectPath)
	if err != nil {
		logging.LogErrorf("parse calendar object path [%s] failed: %s", objectPath, err)
		return
	}

	if value, ok := c.calendars.Load(calendarPath); ok {
		calendar = value.(*Calendar)
	} else {
		err = ErrorCalDavCalendarNotFound
		return
	}

	if value, ok := calendar.Objects.Load(objectID); ok {
		calendarObject = value.(*CalendarObject)
	} else {
		err = ErrorCalDavCalendarObjectNotFound
		return
	}

	return
}

func (c *Calendars) DeleteObject(objectPath string) (calendar *Calendar, calendarObject *CalendarObject, err error) {
	calendarPath, objectID, err := ParseCalendarObjectPath(objectPath)
	if err != nil {
		logging.LogErrorf("parse calendar object path [%s] failed: %s", objectPath, err)
		return
	}

	if value, ok := c.calendars.Load(calendarPath); ok {
		calendar = value.(*Calendar)
	} else {
		err = ErrorCalDavCalendarNotFound
		return
	}

	if value, loaded := calendar.Objects.LoadAndDelete(objectID); loaded {
		calendarObject = value.(*CalendarObject)
	} else {
		err = ErrorCalDavCalendarObjectNotFound
		return
	}

	if err = os.Remove(calendarObject.FilePath); err != nil {
		logging.LogErrorf("remove file [%s] failed: %s", calendarObject.FilePath, err)
		return
	}

	return
}

func (c *Calendars) CreateCalendar(calendarMetaData *caldav.Calendar) (err error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	var calendar *Calendar

	// update map
	if value, ok := c.calendars.Load(calendarMetaData.Path); ok {
		// update map item
		calendar = value.(*Calendar)
		calendar.MetaData = calendarMetaData
	} else {
		// insert map item
		directoryPath, pathErr := c.directoryPath(calendarMetaData.Path)
		if pathErr != nil {
			return pathErr
		}
		calendar = &Calendar{
			Changed:       false,
			DirectoryPath: directoryPath,
			MetaData:      calendarMetaData,
			Objects:       sync.Map{},
		}
		c.calendars.Store(calendarMetaData.Path, calendar)
	}

	var index = -1
	for i, item := range c.calendarsMetaData {
		if item.Path == calendarMetaData.Path {
			index = i
			break
		}
	}

	if index >= 0 {
		// update list
		c.calendarsMetaData[index] = calendarMetaData
	} else {
		// insert list
		c.calendarsMetaData = append(c.calendarsMetaData, calendarMetaData)
	}

	// create calendar directory
	if err = os.MkdirAll(calendar.DirectoryPath, 0755); err != nil {
		logging.LogErrorf("create directory [%s] failed: %s", calendar.DirectoryPath, err)
		return
	}

	// save meta data
	if err = c.saveCalendarsMetaData(); err != nil {
		return
	}

	return
}

func (c *Calendars) ListCalendars() (calendars []caldav.Calendar, err error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	for _, calendar := range c.calendarsMetaData {
		calendars = append(calendars, *calendar)
	}
	return
}

func (c *Calendars) GetCalendar(calendarPath string) (calendar *caldav.Calendar, err error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if value, ok := c.calendars.Load(calendarPath); ok {
		calendar = value.(*Calendar).MetaData
		return
	}

	err = ErrorCalDavCalendarNotFound
	return
}

func (c *Calendars) DeleteCalendar(calendarPath string) (err error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	var calendar *Calendar

	// delete map item
	if value, loaded := c.calendars.LoadAndDelete(calendarPath); loaded {
		calendar = value.(*Calendar)
	}

	// delete list item
	for i, item := range c.calendarsMetaData {
		if item.Path == calendarPath {
			c.calendarsMetaData = append(c.calendarsMetaData[:i], c.calendarsMetaData[i+1:]...)
			break
		}
	}

	// remove address book directory
	if err = os.RemoveAll(calendar.DirectoryPath); err != nil {
		logging.LogErrorf("remove directory [%s] failed: %s", calendar.DirectoryPath, err)
		return
	}

	// save meta data
	if err = c.saveCalendarsMetaData(); err != nil {
		return
	}

	return nil
}

func (c *Calendars) PutCalendarObject(objectPath string, calendarData *ical.Calendar, opts *caldav.PutCalendarObjectOptions) (calendarObject *caldav.CalendarObject, err error) {
	return c.PutCalendarObjectContext(context.Background(), objectPath, calendarData, opts)
}

func (c *Calendars) PutCalendarObjectContext(ctx context.Context, objectPath string, calendarData *ical.Calendar, opts *caldav.PutCalendarObjectOptions) (calendarObject *caldav.CalendarObject, err error) {
	if ctx == nil {
		return nil, errDAVContextUnbound
	}
	if err = ctx.Err(); err != nil {
		return
	}
	c.lock.Lock()
	defer c.lock.Unlock()

	calendarPath, objectID, err := ParseCalendarObjectPath(objectPath)
	if err != nil {
		logging.LogErrorf("parse calendar object path [%s] failed: %s", objectPath, err)
		return
	}

	var calendar *Calendar
	if value, ok := c.calendars.Load(calendarPath); ok {
		calendar = value.(*Calendar)
	} else {
		err = ErrorCalDavCalendarNotFound
		return
	}

	// TODO: 处理 opts.IfNoneMatch (If-None-Match) 与 opts.IfMatch (If-Match)

	var object *CalendarObject
	if value, ok := calendar.Objects.Load(objectID); ok {
		object = value.(*CalendarObject)
		object.Data.Data = calendarData
		object.Changed = true
	} else {
		filePath, pathErr := c.directoryPath(objectPath)
		if pathErr != nil {
			err = pathErr
			return
		}
		object = &CalendarObject{
			Changed:      true,
			FilePath:     filePath,
			CalendarPath: calendarPath,
			Data: &caldav.CalendarObject{
				Data: calendarData,
			},
		}
	}

	err = object.saveContext(ctx, true)
	if err != nil {
		return
	}

	err = object.updateContext(ctx)
	if err != nil {
		return
	}

	calendar.Objects.Store(objectID, object)
	calendarObject = object.Data
	return
}

func (c *Calendars) ListCalendarObjects(calendarPath string, req *caldav.CalendarCompRequest) (calendarObjects []caldav.CalendarObject, err error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.listCalendarObjectsLocked(calendarPath, req)
}

func (c *Calendars) listCalendarObjectsLocked(calendarPath string, req *caldav.CalendarCompRequest) (calendarObjects []caldav.CalendarObject, err error) {
	var calendar *Calendar
	if value, ok := c.calendars.Load(calendarPath); ok {
		calendar = value.(*Calendar)
	} else {
		err = ErrorCalDavCalendarNotFound
		return
	}

	calendar.Objects.Range(func(id any, object any) bool {
		// TODO: filter calendar objects' props and comps
		calendarObjects = append(calendarObjects, *object.(*CalendarObject).Data)
		return true
	})

	return
}

func (c *Calendars) GetCalendarObject(objectPath string, req *caldav.CalendarCompRequest) (calendarObject *caldav.CalendarObject, err error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	_, object, err := c.GetObject(objectPath)
	if err != nil {
		return
	}

	calendarObject = object.Data
	// TODO: filter calendar object's props and comps
	return
}

func (c *Calendars) QueryCalendarObjects(calendarPath string, query *caldav.CalendarQuery) (calendarObjects []caldav.CalendarObject, err error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	calendarObjects, err = c.listCalendarObjectsLocked(calendarPath, &query.CompRequest)
	if err != nil {
		return
	}

	calendarObjects, err = caldav.Filter(query, calendarObjects)
	if err != nil {
		return
	}

	return
}

func (c *Calendars) DeleteCalendarObject(objectPath string) (err error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	_, _, err = c.DeleteObject(objectPath)
	if err != nil {
		return
	}

	return
}

type Calendar struct {
	Changed       bool
	DirectoryPath string
	MetaData      *caldav.Calendar
	Objects       sync.Map // id -> *CalendarObject
}

func (c *Calendar) load() error {
	return c.loadContext(context.Background())
}

func (c *Calendar) loadContext(ctx context.Context) error {
	if ctx == nil {
		return errDAVContextUnbound
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(c.DirectoryPath, 0755); err != nil {
		logging.LogErrorf("create directory [%s] failed: %s", c.DirectoryPath, err)
		return err
	}

	entries, err := os.ReadDir(c.DirectoryPath)
	if err != nil {
		logging.LogErrorf("read dir [%s] failed: %s", c.DirectoryPath, err)
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	var tasks []func(context.Context) error
	for _, entry := range entries {
		if !entry.IsDir() {
			filename := entry.Name()
			ext := util.Ext(filename)
			if ext == ICalendarFileExt {
				filename := filename
				tasks = append(tasks, func(life context.Context) error {
					if err := life.Err(); err != nil {
						return err
					}
					// create & load calendar object
					calendarObjectFilePath := path.Join(c.DirectoryPath, filename)
					calendarObject := &CalendarObject{
						Changed:      false,
						FilePath:     calendarObjectFilePath,
						CalendarPath: c.MetaData.Path,
					}
					if err := calendarObject.loadContext(life); err != nil {
						return err
					}

					id := path.Base(filename)
					c.Objects.Store(id, calendarObject)
					return nil
				})
			}
		}
	}
	return runDAVTasks(ctx, tasks)
}

// save an calendar to multiple *.ics files
func (c *Calendar) save(force bool) error {
	return c.saveContext(context.Background(), force)
}

func (c *Calendar) saveContext(ctx context.Context, force bool) error {
	if ctx == nil {
		return errDAVContextUnbound
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if force || c.Changed {
		// create directory
		if err := os.MkdirAll(c.DirectoryPath, 0755); err != nil {
			logging.LogErrorf("create directory [%s] failed: %s", c.DirectoryPath, err)
			return err
		}

		var tasks []func(context.Context) error
		c.Objects.Range(func(id any, value any) bool {
			object := value.(*CalendarObject)
			tasks = append(tasks, func(life context.Context) error {
				if err := object.saveContext(life, force); err != nil {
					return err
				}
				return object.updateContext(life)
			})
			return true
		})
		if err := runDAVTasks(ctx, tasks); err != nil {
			return err
		}
		c.Changed = false
	}

	return nil
}

type CalendarObject struct {
	Changed      bool
	FilePath     string
	CalendarPath string
	Data         *caldav.CalendarObject
}

func (o *CalendarObject) load() error {
	return o.loadContext(context.Background())
}

func (o *CalendarObject) loadContext(ctx context.Context) error {
	if ctx == nil {
		return errDAVContextUnbound
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// load iCalendar file
	calendarObjectData, err := LoadCalendarObject(o.FilePath)
	if err != nil {
		return err
	}

	// create address object
	o.Data = &caldav.CalendarObject{
		Data: calendarObjectData,
	}

	// update file info
	if err = ctx.Err(); err != nil {
		return err
	}
	err = o.updateContext(ctx)
	if err != nil {
		return err
	}

	o.Changed = false
	return nil
}

// save an object to *.ics file
func (o *CalendarObject) save(force bool) error {
	return o.saveContext(context.Background(), force)
}

func (o *CalendarObject) saveContext(ctx context.Context, force bool) error {
	if ctx == nil {
		return errDAVContextUnbound
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if force || o.Changed {
		var objectData bytes.Buffer

		// encode data
		encoder := ical.NewEncoder(&objectData)
		if err := encoder.Encode(o.Data.Data); err != nil {
			logging.LogErrorf("encode iCalendar [%s] failed: %s", o.Data.Path, err)
			return err
		}

		// create directory
		dirPath := path.Dir(o.FilePath)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			logging.LogErrorf("create directory [%s] failed: %s", dirPath, err)
			return err
		}

		// write file
		if err := os.WriteFile(o.FilePath, objectData.Bytes(), 0755); err != nil {
			logging.LogErrorf("write file [%s] failed: %s", o.FilePath, err)
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		o.Changed = false
	}
	return nil
}

// update file info
func (o *CalendarObject) update() error {
	return o.updateContext(context.Background())
}

func (o *CalendarObject) updateContext(ctx context.Context) error {
	if ctx == nil {
		return errDAVContextUnbound
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	addressFileInfo, err := os.Stat(o.FilePath)
	if err != nil {
		logging.LogErrorf("get file [%s] info failed: %s", o.FilePath, err)
		return err
	}

	o.Data.Path = PathJoinWithSlash(o.CalendarPath, addressFileInfo.Name())
	o.Data.ModTime = addressFileInfo.ModTime()
	o.Data.ContentLength = addressFileInfo.Size()
	o.Data.ETag = FileETag(addressFileInfo)

	return nil
}

type CalDavBackend struct {
	calendars *Calendars
}

func (b *CalDavBackend) owner(ctx context.Context) (*Calendars, error) {
	if ctx == nil {
		return nil, errDAVContextUnbound
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if b == nil || b.calendars == nil {
		return nil, errDAVOwnerUnbound
	}
	return b.calendars, nil
}

func (b *CalDavBackend) load(ctx context.Context) (*Calendars, error) {
	owner, err := b.owner(ctx)
	if err != nil {
		return nil, err
	}
	if err = owner.LoadContext(ctx); err != nil {
		return nil, err
	}
	return owner, ctx.Err()
}

func (b *CalDavBackend) CurrentUserPrincipal(ctx context.Context) (string, error) {
	// logging.LogDebugf("CalDAV CurrentUserPrincipal")
	if _, err := b.owner(ctx); err != nil {
		return "", err
	}
	return CalDavUserPrincipalPath, nil
}

func (b *CalDavBackend) CalendarHomeSetPath(ctx context.Context) (string, error) {
	// logging.LogDebugf("CalDAV CalendarHomeSetPath")
	if _, err := b.owner(ctx); err != nil {
		return "", err
	}
	return CalDavHomeSetPath, nil
}

func (b *CalDavBackend) CreateCalendar(ctx context.Context, calendar *caldav.Calendar) (err error) {
	// logging.LogDebugf("CalDAV CreateCalendar -> calendar: %#v", calendar)
	if calendar == nil {
		return errors.New("calendar_unbound")
	}
	calendar.Path = PathCleanWithSlash(calendar.Path)

	owner, err := b.load(ctx)
	if err != nil {
		return
	}

	err = owner.CreateCalendar(calendar)
	// logging.LogDebugf("CalDAV CreateCalendar <- err: %s", err)
	return
}

func (b *CalDavBackend) ListCalendars(ctx context.Context) (calendars_ []caldav.Calendar, err error) {
	// logging.LogDebugf("CalDAV ListCalendars")
	owner, err := b.load(ctx)
	if err != nil {
		return
	}

	calendars_, err = owner.ListCalendars()
	// logging.LogDebugf("CalDAV ListCalendars <- calendars: %#v, err: %s", calendars_, err)
	return
}

func (b *CalDavBackend) GetCalendar(ctx context.Context, calendarPath string) (calendar *caldav.Calendar, err error) {
	// logging.LogDebugf("CalDAV GetCalendar -> calendarPath: %s", calendarPath)
	calendarPath = PathCleanWithSlash(calendarPath)

	owner, err := b.load(ctx)
	if err != nil {
		return
	}

	calendar, err = owner.GetCalendar(calendarPath)
	// logging.LogDebugf("CalDAV GetCalendar <- calendar: %#v, err: %s", calendar, err)
	return
}

func (b *CalDavBackend) DeleteCalendar(ctx context.Context, calendarPath string) (err error) {
	// logging.LogDebugf("CalDAV DeleteCalendar -> calendarPath: %s", calendarPath)
	calendarPath = PathCleanWithSlash(calendarPath)

	owner, err := b.load(ctx)
	if err != nil {
		return
	}

	err = owner.DeleteCalendar(calendarPath)
	// logging.LogDebugf("CalDAV DeleteCalendar <- err: %s", err)
	return
}

func (b *CalDavBackend) PutCalendarObject(ctx context.Context, objectPath string, calendar *ical.Calendar, opts *caldav.PutCalendarObjectOptions) (calendarObject *caldav.CalendarObject, err error) {
	// logging.LogDebugf("CalDAV PutCalendarObject -> objectPath: %s, opts: %#v", objectPath, opts)
	objectPath = PathCleanWithSlash(objectPath)

	owner, err := b.load(ctx)
	if err != nil {
		return
	}

	calendarObject, err = owner.PutCalendarObjectContext(ctx, objectPath, calendar, opts)
	// logging.LogDebugf("CalDAV PutCalendarObject <- calendarObject: %#v, err: %s", calendarObject, err)
	return
}

func (b *CalDavBackend) ListCalendarObjects(ctx context.Context, calendarPath string, req *caldav.CalendarCompRequest) (calendarObjects []caldav.CalendarObject, err error) {
	// logging.LogDebugf("CalDAV ListCalendarObjects -> calendarPath: %s, req: %#v", calendarPath, req)
	calendarPath = PathCleanWithSlash(calendarPath)

	owner, err := b.load(ctx)
	if err != nil {
		return
	}

	calendarObjects, err = owner.ListCalendarObjects(calendarPath, req)
	// logging.LogDebugf("CalDAV ListCalendarObjects <- calendarObjects: %#v, err: %s", calendarObjects, err)
	return
}

func (b *CalDavBackend) GetCalendarObject(ctx context.Context, objectPath string, req *caldav.CalendarCompRequest) (calendarObject *caldav.CalendarObject, err error) {
	// logging.LogDebugf("CalDAV GetCalendarObject -> objectPath: %s, req: %#v", objectPath, req)
	objectPath = PathCleanWithSlash(objectPath)

	owner, err := b.load(ctx)
	if err != nil {
		return
	}

	calendarObject, err = owner.GetCalendarObject(objectPath, req)
	// logging.LogDebugf("CalDAV GetCalendarObject <- calendarObject: %#v, err: %s", calendarObject, err)
	return
}

func (b *CalDavBackend) QueryCalendarObjects(ctx context.Context, calendarPath string, query *caldav.CalendarQuery) (calendarObjects []caldav.CalendarObject, err error) {
	// logging.LogDebugf("CalDAV QueryCalendarObjects -> calendarPath: %s, query: %#v", calendarPath, query)
	calendarPath = PathCleanWithSlash(calendarPath)

	owner, err := b.load(ctx)
	if err != nil {
		return
	}

	calendarObjects, err = owner.QueryCalendarObjects(calendarPath, query)
	// logging.LogDebugf("CalDAV QueryCalendarObjects <- calendarObjects: %#v, err: %s", calendarObjects, err)
	return
}

func (b *CalDavBackend) DeleteCalendarObject(ctx context.Context, objectPath string) (err error) {
	// logging.LogDebugf("CalDAV DeleteCalendarObject -> objectPath: %s", objectPath)
	objectPath = PathCleanWithSlash(objectPath)

	owner, err := b.load(ctx)
	if err != nil {
		return
	}

	err = owner.DeleteCalendarObject(objectPath)
	// logging.LogDebugf("CalDAV DeleteCalendarObject <- err: %s", err)
	return
}
