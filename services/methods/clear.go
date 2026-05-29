package methods

func Clear() {
	// ntimer.SetTimeout(func() {
	// 	// clear()
	// 	ntimer.SetRepeatTimeTimer(func() {
	// 		clear()
	// 	}, ntimer.RepeatTime{
	// 		Hour: 4,
	// 	}, "Day")
	// }, 400)
}

func clear() {
	// clearUnstoredStaticFile("./static/storage")

	// 未来
	// 清除未使用的File或Folder
}

// 删除没有存储到数据库的静态文件
// func clearUnstoredStaticFile(path string) {
// 	files, err := ioutil.ReadDir(path)
// 	if err != nil {
// 		log.Error(err)
// 		return
// 	}
// 	if len(files) == 0 && path != "./static/storage" {
// 		os.RemoveAll(path)
// 		return
// 	}
// 	for _, f := range files {
// 		if f.IsDir() {
// 			clearUnstoredStaticFile(path + "/" + f.Name())
// 		} else {
// 			sf, err := fileDbx.GetStaticFileWithPath(path, f.Name())
// 			if err != nil {
// 				log.Error(err)
// 				continue
// 			}
// 			if sf == nil {
// 				log.Info("Remove static file -> ", path+"/"+f.Name())
// 				os.Remove(path + "/" + f.Name())
// 			}

// 		}
// 	}
// }
