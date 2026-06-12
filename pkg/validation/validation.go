package validation

import (
	stderrors "errors"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
	enlocale "github.com/go-playground/locales/en"
	zhlocale "github.com/go-playground/locales/zh"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	entranslations "github.com/go-playground/validator/v10/translations/en"
	zhtranslations "github.com/go-playground/validator/v10/translations/zh"

	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
	"github.com/zkw/mini-schedule/backend/pkg/i18n"
)

type Validator struct {
	validate    *validator.Validate
	translators map[i18n.Locale]ut.Translator
}

func New() *Validator {
	validate := validator.New()
	validate.RegisterTagNameFunc(jsonTagName)

	zh := zhlocale.New()
	en := enlocale.New()
	uni := ut.New(zh, zh, en)

	zhTranslator, _ := uni.GetTranslator("zh")
	enTranslator, _ := uni.GetTranslator("en")

	_ = zhtranslations.RegisterDefaultTranslations(validate, zhTranslator)
	_ = entranslations.RegisterDefaultTranslations(validate, enTranslator)

	return &Validator{
		validate: validate,
		translators: map[i18n.Locale]ut.Translator{
			i18n.LocaleZhCN: zhTranslator,
			i18n.LocaleEnUS: enTranslator,
		},
	}
}

func (v *Validator) Struct(value interface{}) error {
	return v.validate.Struct(value)
}

func (v *Validator) InvalidRequest(c *gin.Context, err error) error {
	return apperr.ErrBadRequest(v.Translate(requestLocale(c), err))
}

func (v *Validator) Translate(locale i18n.Locale, err error) string {
	if err == nil {
		return ""
	}

	var validationErrs validator.ValidationErrors
	if !stderrors.As(err, &validationErrs) {
		return i18n.Localize(locale, i18n.KeyInvalidRequest, err.Error())
	}

	translator := v.translators[locale]
	if translator == nil {
		translator = v.translators[i18n.DefaultLocale]
	}

	messages := make([]string, 0, len(validationErrs))
	for _, fieldErr := range validationErrs {
		messages = append(messages, fieldErr.Translate(translator))
	}

	return strings.Join(messages, "; ")
}

func jsonTagName(field reflect.StructField) string {
	name := strings.Split(field.Tag.Get("json"), ",")[0]
	if name == "-" {
		return ""
	}
	if name != "" {
		return name
	}
	return field.Name
}

func requestLocale(c *gin.Context) i18n.Locale {
	if c == nil {
		return i18n.DefaultLocale
	}
	return i18n.FromRequest(c.Request)
}
