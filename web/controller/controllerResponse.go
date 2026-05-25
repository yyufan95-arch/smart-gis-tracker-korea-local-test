package controller

import (
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
)

func ShowView(w http.ResponseWriter, r *http.Request, templateName string, data interface{}) {

	// 指定视图所在路径
	pagePath := filepath.Join("web", "tpl", templateName)

	// 注册模板函数映射：只添加 add，不影响其他模板语法
	funcMap := template.FuncMap{
		"add": func(a, b int) int {
			return a + b
		},
	}

	// 使用 Funcs 注入自定义函数
	resultTemplate, err := template.New(templateName).Funcs(funcMap).ParseFiles(pagePath)
	if err != nil {
		fmt.Printf("创建模板实例错误: %v", err)
		return
	}

	err = resultTemplate.Execute(w, data)
	if err != nil {
		fmt.Printf("在模板中融合数据时发生错误: %v", err)
		return
	}
}

