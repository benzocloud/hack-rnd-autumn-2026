package httpapi

// Public messages are owned by the HTTP adapter. Upstream diagnostics and
// request values must never be interpolated into a user-facing error.
func errorMessage(reason string) string {
	switch reason {
	case "INVALID_CREDENTIALS":
		return "Неверная почта или пароль. Проверьте данные и попробуйте ещё раз."
	case "AUTH_REQUIRED":
		return "Для продолжения войдите в аккаунт. Возможно, срок сессии истёк."
	case "VALIDATION_ERROR":
		return "Проверьте правильность заполнения полей."
	case "INVALID_REQUEST":
		return "Не удалось обработать запрос. Обновите страницу и повторите действие."
	case "CSRF_INVALID":
		return "Не удалось подтвердить безопасность запроса. Обновите страницу и попробуйте ещё раз."
	case "CONSENT_REQUIRED":
		return "Для регистрации нужно согласие на обработку персональных данных."
	case "CONSENT_DOCUMENT_CHANGED":
		return "Текст согласия обновился. Ознакомьтесь с новой версией и подтвердите согласие."
	case "EMAIL_UNVERIFIED":
		return "Подтвердите почту, чтобы продолжить."
	case "CODE_INVALID_OR_EXPIRED":
		return "Неверный код подтверждения или срок его действия истёк. Проверьте код либо запросите новый."
	case "TOKEN_INVALID_OR_EXPIRED":
		return "Ссылка недействительна или срок её действия истёк. Запросите новую ссылку."
	case "FORBIDDEN":
		return "У вас недостаточно прав для этого действия."
	case "MEMBERSHIP_REVOKED":
		return "Доступ к компании отозван. Обратитесь к её владельцу."
	case "NOT_FOUND":
		return "Запрашиваемые данные не найдены или недоступны."
	case "INVALID_CURSOR":
		return "Не удалось загрузить следующую страницу списка. Обновите список и попробуйте ещё раз."
	case "RATE_LIMITED":
		return "Слишком много попыток. Подождите немного и попробуйте ещё раз."
	case "VERSION_CONFLICT":
		return "Данные уже изменились. Обновите страницу перед повторным сохранением."
	case "IDEMPOTENCY_CONFLICT":
		return "Этот запрос уже использован для другого действия. Обновите страницу и повторите операцию."
	case "INVITATION_NOT_AVAILABLE":
		return "Приглашение недействительно, уже использовано или срок его действия истёк. Попросите владельца отправить новое."
	case "INVITATION_STATE_CONFLICT":
		return "Состояние приглашения изменилось. Обновите список приглашений."
	case "COMPANY_CREATION_IN_PROGRESS":
		return "Компания ещё создаётся. Подождите немного и обновите страницу."
	case "FACTUAL_SOURCE_CONFLICT":
		return "Источник истории счёта не допускает эту операцию. Проверьте выбранный способ ввода данных."
	case "SOURCE_BINDING_CONFLICT":
		return "Счёт уже связан с другим источником данных. Проверьте настройки интеграции."
	case "BALANCE_OBSERVATION_CONFLICT":
		return "Остаток на выбранную дату уже сохранён с другими данными. Обновите сведения о счёте."
	case "PAYMENT_STATE_CONFLICT":
		return "Платёж уже изменён или исполнен. Обновите календарь платежей."
	case "IMPORT_STATE_CONFLICT":
		return "Импорт уже обработан или его состояние изменилось. Обновите страницу импорта."
	case "IMPORT_PREVIEW_STALE":
		return "После проверки импорта данные счёта изменились. Повторите проверку перед загрузкой."
	case "IMPORT_WARNINGS_NOT_ACKNOWLEDGED":
		return "Перед импортом ознакомьтесь с предупреждениями и подтвердите их."
	case "BALANCE_AS_OF_UNAVAILABLE":
		return "Нет остатка на выбранную дату. Укажите остаток счёта для расчёта прогноза."
	case "INSUFFICIENT_HISTORY":
		return "Недостаточно подтверждённой истории для прогноза. Для текущей модели нужны 90 дней данных."
	case "UNSUPPORTED_MODEL_CONTEXT":
		return "Выбранные параметры расчёта не поддерживаются моделью. Попробуйте прогноз на 30 дней."
	case "FORECAST_NOT_READY":
		return "Прогноз ещё не готов. Дождитесь завершения расчёта."
	case "FORECAST_NOT_COMPARABLE":
		return "Эти прогнозы нельзя сравнить: параметры или исходные данные различаются."
	case "ACTUALS_NOT_AVAILABLE":
		return "Фактические данные за выбранный период пока недоступны."
	case "SCENARIO_NOT_SUPPORTED":
		return "Этот сценарий пока недоступен для выбранного прогноза."
	case "SCENARIO_TARGET_CONFLICT":
		return "Данные сценария изменились или выбранный объект недоступен. Обновите сценарий."
	case "PAYLOAD_TOO_LARGE":
		return "Слишком большой объём данных. Уменьшите размер загрузки и попробуйте ещё раз."
	case "DEPENDENCY_UNAVAILABLE":
		return "Сервис временно недоступен. Попробуйте ещё раз немного позже."
	default:
		return "Произошла внутренняя ошибка. Попробуйте позже; если ошибка повторится, сообщите поддержке номер запроса."
	}
}

func fieldMessage(field string) string {
	switch field {
	case "email":
		return "Укажите почту в формате name@example.ru."
	case "password", "new_password":
		return "Проверьте пароль и требования к его формату."
	case "display_name":
		return "Укажите корректное имя."
	case "personal_data_consent":
		return "Подтвердите согласие с актуальным текстом обработки персональных данных."
	case "code":
		return "Введите код из шести цифр."
	case "challenge_id":
		return "Запросите новый код подтверждения."
	case "cutoff_date":
		return "Проверьте дату, на которую выполняется расчёт."
	case "horizon_days":
		return "Выберите поддерживаемый период прогноза — 30 дней."
	case "account_id":
		return "Проверьте выбранный счёт компании."
	default:
		return "Запросите новую ссылку и повторите действие."
	}
}
