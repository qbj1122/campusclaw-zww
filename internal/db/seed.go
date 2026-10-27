package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// Seed installs the demo classes, accounts, materials, skills and agents. It is
// idempotent: existing rows are never overwritten, so uploaded data survives a
// restart. Passwords are hashed, never stored in plaintext.
func Seed(ctx context.Context, db *sql.DB, uploadDir, password string) error {
	if password == "" {
		return fmt.Errorf("seed password is empty")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return err
	}
	classA, err := ensureClass(ctx, db, "班级A")
	if err != nil {
		return err
	}
	classB, err := ensureClass(ctx, db, "班级B")
	if err != nil {
		return err
	}
	teacherA, err := ensureUser(ctx, db, "teacher_a", "张老师", "teacher", classA, string(hash))
	if err != nil {
		return err
	}
	teacherB, err := ensureUser(ctx, db, "teacher_b", "王老师", "teacher", classB, string(hash))
	if err != nil {
		return err
	}
	if _, err := ensureUser(ctx, db, "student_a1", "李明", "student", classA, string(hash)); err != nil {
		return err
	}
	if _, err := ensureUser(ctx, db, "student_b1", "王芳", "student", classB, string(hash)); err != nil {
		return err
	}

	if _, err := ensureDefaultPrompt(ctx, db, classA, "你是班级A的学科助手。先确认问题属于哪一学科，再依据本班材料回答；没有依据时明确说明，不编造出处。"); err != nil {
		return err
	}
	if _, err := ensureDefaultPrompt(ctx, db, classB, "你是班级B的学科助手。先确认问题属于哪一学科，再依据本班材料回答；没有依据时明确说明，不编造出处。"); err != nil {
		return err
	}

	mA, err := ensureMaterial(ctx, db, classA, teacherA, uploadDir, "数学自学笔记", mathNote)
	if err != nil {
		return err
	}
	cA, err := ensureMaterial(ctx, db, classA, teacherA, uploadDir, "语文自学笔记", chineseNote)
	if err != nil {
		return err
	}
	mB, err := ensureMaterial(ctx, db, classB, teacherB, uploadDir, "B班物理笔记", physicsNoteB)
	if err != nil {
		return err
	}
	_ = mB

	mathSkill, err := ensureSkill(ctx, db, classA, "seed-math", mathSkillFile)
	if err != nil {
		return err
	}
	chineseSkill, err := ensureSkill(ctx, db, classA, "seed-chinese", chineseSkillFile)
	if err != nil {
		return err
	}
	if _, err := ensureSkill(ctx, db, classB, "seed-b-math", mathSkillFile); err != nil {
		return err
	}

	if _, err := ensureAgent(ctx, db, classA, "数学解题助手", "你是班级A的数学解题助手。先判断学生卡在哪一步，只给下一步提示，不直接写出最终得数。回答优先引用本班数学材料。", mathSkill, mA); err != nil {
		return err
	}
	if _, err := ensureAgent(ctx, db, classA, "语文解题助手", "你是班级A的语文解题助手。接住学生已经写出的内容，提示他往前看哪一句，不直接给出答案。回答优先引用本班语文材料。", chineseSkill, cA); err != nil {
		return err
	}
	if _, err := ensureHandout(ctx, db, classA, "第1讲 一元一次方程"); err != nil {
		return err
	}
	if _, err := ensureAssignment(ctx, db, classA, "第1周作业：去括号练习"); err != nil {
		return err
	}
	return nil
}

func ensureClass(ctx context.Context, db *sql.DB, name string) (int64, error) {
	var id int64
	err := db.QueryRowContext(ctx, `SELECT id FROM classes WHERE name=?`, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	res, err := db.ExecContext(ctx, `INSERT INTO classes(name) VALUES(?)`, name)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func ensureUser(ctx context.Context, db *sql.DB, username, display, role string, classID int64, hash string) (int64, error) {
	var id int64
	err := db.QueryRowContext(ctx, `SELECT id FROM users WHERE username=?`, username).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	res, err := db.ExecContext(ctx, `INSERT INTO users(username,display_name,role,class_id,password_hash) VALUES(?,?,?,?,?)`,
		username, display, role, classID, hash)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func ensureDefaultPrompt(ctx context.Context, db *sql.DB, classID int64, prompt string) (bool, error) {
	res, err := db.ExecContext(ctx, `INSERT IGNORE INTO class_settings(class_id,prompt) VALUES(?,?)`, classID, prompt)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func ensureMaterial(ctx context.Context, db *sql.DB, classID, teacherID int64, uploadDir, title, body string) (int64, error) {
	var id int64
	err := db.QueryRowContext(ctx, `SELECT id FROM materials WHERE class_id=? AND title=? LIMIT 1`, classID, title).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	if err := os.MkdirAll(uploadDir, 0o750); err != nil {
		return 0, err
	}
	name := fmt.Sprintf("seed-%d-%s.txt", classID, sanitizeName(title))
	path := filepath.Join(uploadDir, name)
	if err := os.WriteFile(path, []byte(body), 0o640); err != nil {
		return 0, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO materials(class_id,uploaded_by,title,file_path,mime_type,size_bytes) VALUES(?,?,?,?,?,?)`,
		classID, teacherID, title, path, "text/markdown", len(body))
	if err != nil {
		return 0, err
	}
	id, err = res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO knowledge_entries(material_id,class_id,body_text) VALUES(?,?,?)`, id, classID, body); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

type parsedSkill struct {
	Name        string
	TriggerText string
	SubjectText string
	Tools       []string
	Enabled     bool
	Description string
}

func parseSeedSkill(text string) parsedSkill {
	ps := parsedSkill{Enabled: true}
	parts := strings.SplitN(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n", 2)
	meta := parts[0]
	if len(parts) > 1 {
		ps.Description = strings.TrimSpace(parts[1])
	}
	for _, line := range strings.Split(meta, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "：")
		if !ok {
			k, v, ok = strings.Cut(line, ":")
		}
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "名称":
			ps.Name = strings.TrimSpace(v)
		case "适用条件":
			ps.TriggerText = strings.TrimSpace(v)
		case "资料":
			ps.SubjectText = strings.TrimSpace(v)
		case "工具":
			ps.Tools = splitList(v)
		case "启用":
			ps.Enabled = strings.TrimSpace(v) == "是" || strings.EqualFold(strings.TrimSpace(v), "true")
		}
	}
	return ps
}

func splitList(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '，' || r == '、' || r == ';' || r == '；' || r == '|'
	})
}

func ensureSkill(ctx context.Context, db *sql.DB, classID int64, seedKey, file string) (int64, error) {
	var id int64
	err := db.QueryRowContext(ctx, `SELECT id FROM skills WHERE seed_key=?`, seedKey).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	ps := parseSeedSkill(file)
	res, err := db.ExecContext(ctx, `INSERT INTO skills(class_id,name,trigger_text,description,tools_json,source_text,subject_text,enabled,seed_key)
		VALUES(?,?,?,?,?,?,?,?,?)`, classID, ps.Name, ps.TriggerText, ps.Description, "["+quoteList(ps.Tools)+"]", file, ps.SubjectText, ps.Enabled, seedKey)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func quoteList(items []string) string {
	q := make([]string, 0, len(items))
	for _, s := range items {
		q = append(q, `"`+strings.ReplaceAll(s, `"`, `\"`)+`"`)
	}
	return strings.Join(q, ",")
}

func ensureAgent(ctx context.Context, db *sql.DB, classID int64, name, prompt string, skillID, materialID int64) (int64, error) {
	var id int64
	err := db.QueryRowContext(ctx, `SELECT id FROM agents WHERE class_id=? AND name=?`, classID, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO agents(class_id,name,prompt,enabled) VALUES(?,?,?,1)`, classID, name, prompt)
	if err != nil {
		return 0, err
	}
	id, _ = res.LastInsertId()
	if _, err := tx.ExecContext(ctx, `INSERT INTO agent_skills(agent_id,skill_id) VALUES(?,?)`, id, skillID); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO agent_materials(agent_id,material_id) VALUES(?,?)`, id, materialID); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func ensureHandout(ctx context.Context, db *sql.DB, classID int64, title string) (bool, error) {
	res, err := db.ExecContext(ctx, `INSERT INTO handouts(class_id,title,body)
		SELECT ?,?,? FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM handouts WHERE class_id=? AND title=?)`,
		classID, title, "占位讲义，后续迭代补充。", classID, title)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func ensureAssignment(ctx context.Context, db *sql.DB, classID int64, title string) (bool, error) {
	res, err := db.ExecContext(ctx, `INSERT INTO assignments(class_id,title,description)
		SELECT ?,?,? FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM assignments WHERE class_id=? AND title=?)`,
		classID, title, "占位作业，后续迭代补充。", classID, title)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func sanitizeName(s string) string {
	r := strings.NewReplacer("/", "-", "\\", "-", " ", "-")
	return r.Replace(s)
}

const mathSkillFile = `名称：数学解题引导
适用条件：方程,去括号,分数,通分,面积,应用题,未知数,行程
资料：数学
工具：knowledge_search
启用：是

学生正在自己做数学作业，往往已经写了一步。先看懂他卡在哪一步，只指出下一步该检查什么，例如去括号有没有乘到每一项、分母不同要不要先通分、该设哪个未知数。不要算出最终得数，不要把整道题做完。依据只用数学自学笔记，并标出材料标题和位置。`

const chineseSkillFile = `名称：语文解题引导
适用条件：春晓,默写,阅读,中心,作文,开头,古诗,词语
资料：语文
工具：knowledge_search
启用：是

学生正在自己背诵、做阅读或改作文，卡在某一句。先接住他已经写出的那一句，告诉他往前看哪一句、找哪类句子。不要把诗的下一句、中心句或改好的段落直接写出来。依据只用语文自学笔记，并标出材料标题和位置。`

const mathNote = `# 数学自学笔记

## 第一章 一元一次方程

解一元一次方程的一般步骤是：去分母、去括号、移项、合并同类项、系数化为1。去括号时，括号前面的系数要乘到括号里面的每一项，括号前面是负号时，去掉括号后每一项都要改变符号。例如 3(x-2)=9，先去括号得到 3x-6=9，再移项得到 3x=15，最后系数化为1得到 x=5。

## 第二章 行程问题

行程问题的基本关系是路程等于速度乘以时间。相遇问题中，两人从两地同时出发相向而行，路程和等于两地距离；追及问题中，路程差等于初始距离。设未知数时，先问自己题目要求的是哪一个量，再把其余量用这个未知数表示。画线段图可以避免把相遇与追及的条件混淆。

## 第三章 分数与通分

分母不同的分数相加减，先通分再计算。通分的关键是找到分母的最小公倍数。去括号与通分同时出现时，先处理括号，再统一分母，避免漏乘。检查答案时可以把结果代回原式，看看两边是否相等。

## 第四章 面积与几何

长方形面积等于长乘宽，三角形面积等于底乘高除以二。遇到组合图形，先把它拆成几个基本图形，分别计算后再加减。单位不统一时先换算单位，再代入公式。

数学作业的检查顺序：先看符号，再看去括号有没有乘全，最后把答案代回原题。只看出答案对不对，无法发现中间步骤的漏洞。`

const chineseNote = `# 语文自学笔记

## 第一章 古诗默写

《春晓》的作者是孟浩然，全诗写春夜睡眠、醒来听鸟、回忆风雨落花。默写时先想画面，再想诗句顺序，不要只背第一句。易错字要单独抄写，例如“晓”是日字旁，“眠”是目字旁。

## 第二章 阅读理解

阅读题先找题干中的关键词，再回到原文定位。说明中心句通常在段首或段尾；如果段落先举例后总结，中心句多在段尾。回答“为什么”时，要同时写出原因和结果，不能只抄原文一句。

## 第三章 作文开头

作文开头可以写场景、写冲突或写一个具体动作。不要一上来喊口号，也不要把题目再抄一遍。开头段落控制在三到四行，给后文留出展开空间。修改时先看开头有没有把读者带进具体情境。

## 第四章 词语积累

词语题要结合上下文判断，不要只看字面。近义词辨析可以从程度、范围、感情色彩三个方面比较。遇到不认识的词，先看它前后搭配，再查工具书确认。`

const physicsNoteB = `# B班物理笔记

## 第一章 力与运动

牛顿第一定律说明物体在不受外力时保持静止或匀速直线运动。计算合力时先规定正方向，再把我方向相反的量写成负数。

## 第二章 B班专属例题

本班独有的例题代号是量子纠缠草莓。该代号只出现在B班材料中，用于验证班级隔离。若A班用户能够检索到量子纠缠草莓，说明检索侧的班级过滤失效。

## 第三章 单位换算

米与厘米之间相差一百倍，千克与克之间相差一千倍。换算时先写单位关系，再移动小数点，最后检查数量级是否合理。`
