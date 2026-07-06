package db

import "testing"

func TestDefaultMenusIncludeQuizQuestionBank(t *testing.T) {
	var found bool
	for _, menu := range defaultMenus {
		if menu.Name == "CustomerQuizQuestions" {
			found = true
			if menu.Path != "/customer/quiz-questions" || menu.Component != "/quiz/questions" {
				t.Fatalf("unexpected quiz question menu route: %+v", menu)
			}
			if menu.AuthCode != "Website:Write" || menu.Type != "menu" {
				t.Fatalf("unexpected quiz question menu auth/type: %+v", menu)
			}
		}
	}
	if !found {
		t.Fatal("default menus should include quiz question bank page")
	}
}
