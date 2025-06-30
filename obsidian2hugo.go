package main

import (
	"crypto/md5"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Аргументы командной строки
var (
	notesDir        = flag.String("notes-dir", "", "Абсолютный путь к каталогу с вашими заметками Obsidian (.md файлы).")
	attachmentsDir  = flag.String("attachments-dir", "", "Абсолютный путь к каталогу, где Obsidian хранит все вложения.")
	hugoPostsDir    = flag.String("hugo-posts-dir", "", "Абсолютный путь к целевому каталогу для контента Hugo.")
	filterTag       = flag.String("filter-tag", "blog", "Тег, по которому отбираются заметки.")
	removeFilterTag = flag.Bool("remove-filter-tag", false, "Если указано, тег фильтрации будет удален из финального списка тегов.")
	logLevel        = flag.String("log-level", "INFO", "Уровень логирования (DEBUG, INFO, WARNING, ERROR).")
)

// Пользовательский тип для обработки списка строковых значений из флагов
type stringSlice []string

func (s *stringSlice) String() string {
	return strings.Join(*s, " ")
}

func (s *stringSlice) Set(value string) error {
	*s = append(*s, value)
	return nil
}

var excludeDirs stringSlice

// Уровни логирования
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARNING
	ERROR
)

var currentLogLevel LogLevel

// setLogLevel устанавливает текущий уровень логирования.
func setLogLevel(level string) {
	switch strings.ToUpper(level) {
	case "DEBUG":
		currentLogLevel = DEBUG
	case "INFO":
		currentLogLevel = INFO
	case "WARNING":
		currentLogLevel = WARNING
	case "ERROR":
		currentLogLevel = ERROR
	default:
		currentLogLevel = INFO
	}
	log.SetFlags(0) // Убираем стандартные префиксы времени и даты
	log.SetOutput(os.Stdout)
}

// logf выводит сообщение в лог, если его уровень не ниже текущего.
func logf(level LogLevel, format string, v ...interface{}) {
	if level >= currentLogLevel {
		prefix := fmt.Sprintf("[%s] ", strings.ToUpper(level.String()))
		log.Printf(prefix+format, v...)
	}
}

// String для LogLevel для красивого вывода
func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARNING:
		return "WARNING"
	case ERROR:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Регулярные выражения
var (
	// Паттерн для поиска блока YAML Front Matter в начале файла.
	frontMatterPattern = regexp.MustCompile(`(?s)^---\s*\n(.*?)\n---\s*`)
	// Паттерн для поиска вложений Obsidian.
	attachmentPattern = regexp.MustCompile(`!\[\[(.*?)\]\]`)
	// Паттерн для поиска вики-ссылок (не должен захватывать вложения).
	wikilinkPattern = regexp.MustCompile(`\[\[(.*?)\]\]`)
)

func main() {
	// Описание для --exclude-dirs
	flag.Var(&excludeDirs, "exclude-dirs", "Список имен каталогов для исключения из сканирования (через пробел).")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Использование: %s [аргументы]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Конвертирует заметки Obsidian в формат Hugo Page Bundle.\n\n")
		fmt.Fprintf(os.Stderr, "Аргументы:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	setLogLevel(*logLevel)

	if *notesDir == "" || *attachmentsDir == "" || *hugoPostsDir == "" {
		flag.Usage()
		logf(ERROR, "Ошибка: Аргументы --notes-dir, --attachments-dir и --hugo-posts-dir являются обязательными.")
		os.Exit(1)
	}

	if err := processNotes(); err != nil {
		logf(ERROR, "Не удалось обработать заметки: %v", err)
		os.Exit(1)
	}
}

// processNotes сканирует и обрабатывает все заметки.
func processNotes() error {
	logf(INFO, "Рекурсивно сканирую заметки в: %s", *notesDir)
	if len(excludeDirs) > 0 {
		logf(INFO, "Исключаю каталоги: %v", excludeDirs)
	}

	absExcludePaths := make(map[string]struct{})
	for _, dir := range excludeDirs {
		absPath, err := filepath.Abs(filepath.Join(*notesDir, dir))
		if err == nil {
			absExcludePaths[absPath] = struct{}{}
		}
	}

	err := filepath.Walk(*notesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Пропускаем исключенные каталоги
		if info.IsDir() {
			if _, excluded := absExcludePaths[path]; excluded {
				logf(DEBUG, "Пропускаю исключенный каталог: %s", path)
				return filepath.SkipDir
			}
			return nil
		}

		// Обрабатываем только .md файлы
		if !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}

		logf(INFO, "--- Проверяю заметку: %s ---", strings.TrimPrefix(path, *notesDir+"/"))
		return processNoteFile(path)
	})

	if err != nil {
		return err
	}

	logf(INFO, "--- Обработка завершена. ---")
	return nil
}

// processNoteFile обрабатывает один файл заметки.
func processNoteFile(path string) error {
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("не удалось прочитать заметку %s: %w", path, err)
	}
	fullContent := string(contentBytes)

	properties, content, err := parseNoteContent(fullContent)
	if err != nil {
		logf(WARNING, "Не удалось разобрать front matter для %s: %v. Пропускаю.", path, err)
		return nil // Не прерываем весь процесс из-за одной плохой заметки
	}

	// --- ПРОВЕРКА ТЕГА ---
	tags, ok := properties["tags"]
	if !ok {
		logf(DEBUG, "Пропускаю заметку '%s', так как у нее нет тегов.", filepath.Base(path))
		return nil
	}

	var tagsList []string
	switch v := tags.(type) {
	case []interface{}:
		for _, t := range v {
			if tagStr, ok := t.(string); ok {
				tagsList = append(tagsList, tagStr)
			}
		}
	case string:
		for _, tagStr := range strings.Split(v, ",") {
			tagsList = append(tagsList, strings.TrimSpace(tagStr))
		}
	}

	found := false
	for _, t := range tagsList {
		if t == *filterTag {
			found = true
			break
		}
	}

	if !found {
		logf(DEBUG, "Пропускаю заметку '%s', так как у нее нет тега '%s'.", filepath.Base(path), *filterTag)
		return nil
	}

	logf(INFO, "Обрабатываю заметку: %s (найден тег '%s')", filepath.Base(path), *filterTag)

	// --- ОБНОВЛЕНИЕ ТЕГОВ ---
	if *removeFilterTag {
		var updatedTags []string
		for _, t := range tagsList {
			if t != *filterTag {
				updatedTags = append(updatedTags, t)
			}
		}
		if len(updatedTags) > 0 {
			properties["tags"] = updatedTags
		} else {
			delete(properties, "tags")
		}
		logf(DEBUG, "Удаляю тег '%s' из списка тегов.", *filterTag)
	}

	// --- ЛОГИКА УПРАВЛЕНИЯ FRONT MATTER ---
	if _, ok := properties["title"]; !ok {
		title := strings.TrimSuffix(filepath.Base(path), ".md")
		properties["title"] = title
		logf(DEBUG, "Свойство 'title' не найдено. Установлено: '%s'", title)
	}

	if _, ok := properties["date"]; !ok {
		date := time.Now().Format(time.RFC3339)
		properties["date"] = date
		logf(DEBUG, "Свойство 'date' не найдено. Установлено: '%s'", date)
	}

	// --- СОЗДАНИЕ PAGE BUNDLE ---
	bundleDirName := strings.TrimSuffix(filepath.Base(path), ".md")
	targetBundleDir := filepath.Join(*hugoPostsDir, bundleDirName)
	if err := os.MkdirAll(targetBundleDir, 0755); err != nil {
		return fmt.Errorf("не удалось создать каталог поста %s: %w", targetBundleDir, err)
	}
	logf(INFO, "Создан/обновлен каталог поста: %s", targetBundleDir)

	// --- ОБРАБОТКА ВЛОЖЕНИЙ ---
	content, err = processAttachments(content, targetBundleDir)
	if err != nil {
		return err
	}

	// --- ОБРАБОТКА ВИКИ-ССЫЛОК ---
	if wikilinkPattern.MatchString(content) {
		logf(INFO, "Обновляю вики-ссылки в тексте (удаляю квадратные скобки)...")
		content = wikilinkPattern.ReplaceAllString(content, "$1")
	}

	// --- ЗАПИСЬ РЕЗУЛЬТАТА ---
	finalContent, err := writeFinalNote(properties, content)
	if err != nil {
		return err
	}

	targetNotePath := filepath.Join(targetBundleDir, "index.md")
	if err := os.WriteFile(targetNotePath, []byte(finalContent), 0644); err != nil {
		return fmt.Errorf("не удалось записать итоговую заметку %s: %w", targetNotePath, err)
	}

	logf(INFO, "Заметка сохранена как: %s", targetNotePath)
	return nil
}

// parseNoteContent извлекает YAML front matter и основное содержимое.
func parseNoteContent(fullContent string) (map[string]interface{}, string, error) {
	matches := frontMatterPattern.FindStringSubmatch(fullContent)
	if len(matches) < 2 {
		// Front matter не найден, возвращаем пустые свойства и полный контент
		return make(map[string]interface{}), fullContent, nil
	}

	yamlContent := matches[1]
	noteBody := strings.TrimSpace(fullContent[len(matches[0]):])

	var properties map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &properties); err != nil {
		return nil, "", fmt.Errorf("ошибка парсинга YAML: %w", err)
	}
	if properties == nil {
		properties = make(map[string]interface{})
	}

	return properties, noteBody, nil
}

// processAttachments обрабатывает вложения в тексте заметки.
func processAttachments(content, targetBundleDir string) (string, error) {
	matches := attachmentPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return content, nil
	}

	logf(INFO, "Обновляю ссылки на вложения в тексте...")
	newContent := content
	for _, match := range matches {
		originalLinkText := match[0]
		originalFilename := match[1]

		sourceAttachmentPath := filepath.Join(*attachmentsDir, originalFilename)
		if _, err := os.Stat(sourceAttachmentPath); os.IsNotExist(err) {
			logf(WARNING, "Вложение '%s' не найдено в %s", originalFilename, *attachmentsDir)
			continue
		}

		md5Hash, err := calculateMD5(sourceAttachmentPath)
		if err != nil {
			logf(WARNING, "Не удалось вычислить MD5 для %s: %v", sourceAttachmentPath, err)
			continue
		}

		extension := filepath.Ext(sourceAttachmentPath)
		newFilename := fmt.Sprintf("%s%s", md5Hash, extension)
		targetAttachmentPath := filepath.Join(targetBundleDir, newFilename)

		if err := copyFile(sourceAttachmentPath, targetAttachmentPath); err != nil {
			logf(WARNING, "Не удалось скопировать вложение '%s' -> '%s': %v", originalFilename, newFilename, err)
			continue
		}
		logf(DEBUG, "Копирую вложение: '%s' -> '%s'", originalFilename, newFilename)

		newLinkText := fmt.Sprintf("![](%s)", newFilename)
		newContent = strings.Replace(newContent, originalLinkText, newLinkText, -1)
	}
	return newContent, nil
}

// calculateMD5 вычисляет MD5-хэш файла.
func calculateMD5(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// copyFile копирует файл из src в dst.
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// writeFinalNote собирает итоговый файл с front matter и контентом.
func writeFinalNote(properties map[string]interface{}, content string) (string, error) {
	// Marshal делает сортировку ключей по умолчанию, что нам не нужно.
	// Чтобы сохранить порядок, можно было бы использовать yaml.Node, но для простоты оставим так.
	yamlHeader, err := yaml.Marshal(properties)
	if err != nil {
		return "", fmt.Errorf("не удалось преобразовать front matter в YAML: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.Write(yamlHeader)
	sb.WriteString("---\n\n")
	sb.WriteString(content)

	return sb.String(), nil
}
