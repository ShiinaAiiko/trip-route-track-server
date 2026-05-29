package conf

import (
	"github.com/ShiinaAiiko/nyanya-trip-route-track/server/protos"
	"github.com/cherrai/nyanyago-utils/fileStorageDB"
)

var (
	// ConfigFS      *fileStorage.FileStorage[fileStorage.H]
	// DeviceTokenFS *fileStorage.FileStorage[*typings.DeviceTokenInfo]
	// BackupsFS     *fileStorage.FileStorage[*protos.BackupItem]

	FsDB *fileStorageDB.DB

	GlobalFsDB    *fileStorageDB.Model[any]
	AISessionFsDB *fileStorageDB.Model[[]*protos.ChatContextItem]
	// TestFS *fileStorageDB.Model[fileStorageDB.H]
	// DeviceTokenFS *fileStorageDB.Model[*typings.DeviceTokenInfo]
	// BackupsFS     *fileStorageDB.Model[*protos.BackupItem]
)

func InitFsDB() {
	// if runtime.GOOS == "windows" {
	// 	home := os.Getenv("HOMEDRIVE") + os.Getenv("HOMEPATH")
	// 	if home == "" {
	// 		home = os.Getenv("USERPROFILE")
	// 	}
	// }

	db, err := fileStorageDB.Open("./fsdb/fs.db")
	if err != nil {
		log.Error(err)
	}
	FsDB = db

	if GlobalFsDB == nil {
		db, err := fileStorageDB.CreateModel[any](FsDB, "Global")
		if err != nil {
			log.Error(err)
		} else {
			GlobalFsDB = db
		}
	}
	if AISessionFsDB == nil {
		db, err := fileStorageDB.CreateModel[[]*protos.ChatContextItem](FsDB, "AISession")
		if err != nil {
			log.Error(err)
		} else {
			AISessionFsDB = db
		}
	}

	// type AAAA struct {
	// 	BBBBB string
	// }

	// TestFS, err := fileStorageDB.CreateModel[AAAA](db, "config")
	// if err != nil {
	// 	log.Error(err)
	// 	return
	// }

	// ntimer.SetTimeout(func() {

	// 	// TestFS.Set("a", &AAAA{
	// 	// 	BBBBB: "33333",
	// 	// }, 0)

	// 	val, err := TestFS.Get("a")
	// 	log.Info(val, err)
	// 	log.Info(val.Value())
	// 	ntimer.SetTimeout(func() {
	// 		val, err := TestFS.Get("a")
	// 		log.Info(val, err)
	// 		log.Info(val.Value().BBBBB)
	// 	}, 3000)
	// }, 1000)

	// DeviceTokenFS, err = fileStorageDB.CreateModel[*typings.DeviceTokenInfo](db, "deviceToken")
	// if err != nil {
	// 	log.Error(err)
	// 	return
	// }

	// BackupsFS, err = fileStorageDB.CreateModel[*protos.BackupItem](db, "backups")
	// if err != nil {
	// 	log.Error(err)
	// 	return
	// }

	// ConfigFS = fileStorage.New[fileStorage.H](&fileStorage.FileStorageOptions{
	// 	Label:       "config",
	// 	StoragePath: DatabasePath,
	// })
	// DeviceTokenFS = fileStorage.New[*typings.DeviceTokenInfo](&fileStorage.FileStorageOptions{
	// 	Label:       "deviceToken",
	// 	StoragePath: DatabasePath,
	// })
	// BackupsFS = fileStorage.New[*protos.BackupItem](&fileStorage.FileStorageOptions{
	// 	Label:       "backups",
	// 	StoragePath: DatabasePath,
	// })
}

// ntimer.SetTimeout(func() {
// 	user, _ := user.Current()
// 	type A struct {
// 		CC string
// 	}
// 	systemConfig := fileStorage.New[string](&fileStorage.FileStorageOptions{
// 		Label:       "systemConfig",
// 		StoragePath: path.Join(user.HomeDir, "/.config/meow-backups/s"),
// 	})
// 	log.Info(systemConfig)

// 	// systemConfig.Set("ce1s", "cesaaaaaaaa", 0)
// 	// systemConfig.Set("ces", "cesaaaaaaaa", 5000*time.Second)

// 	// systemConfig.Set("ces3", &A{
// 	// 	CC: "sasa3",
// 	// }, 5000*time.Second)
// 	// log.Info(systemConfig.Get("ces"))
// 	// vvvv, _ := systemConfig.Get("ces3")
// 	// log.Info(vvvv, vvvv.CC)
// 	log.Info(systemConfig.Keys())
// 	log.Info(systemConfig.Values())
// 	// systemConfig.Delete("ces")
// }, 1000)
