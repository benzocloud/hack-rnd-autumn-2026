import Image from "next/image";

import appLinks from "@/helpers/appLinks";
import Footer from "@/widgets/Footer/Footer";
import Icon from "@/widgets/Icon/Icon";

import ForecastDemo from "./ForecastDemo";
import Header from "./Header";
import Reveal from "./Reveal";
import ScenarioDemo from "./ScenarioDemo";
import styles from "./style.module.scss";


const steps = [
    { number: "01", icon: "building", title: "Начните с вашего бизнеса", text: "Создайте компанию и укажите остаток денег на известную дату." },
    { number: "02", icon: "file-upload", title: "Добавьте денежные потоки", text: "Загрузите историю операций и внесите будущие платежи и поступления." },
    { number: "03", icon: "chart-line", title: "Посмотрите вперёд", text: "Выберите горизонт 30, 60 или 90 дней и изучите прогноз остатка." },
    { number: "04", icon: "adjustments", title: "Проверьте своё решение", text: "Сравните сценарии, чтобы понять, как изменение платежей повлияет на деньги." },
];

const questions = [
    { title: "Какие данные нужны для начала?", text: "Создайте компанию, укажите остаток на счёте и дату, на которую он известен. Затем добавьте историю операций, ожидаемые поступления и обязательные платежи. Чем полнее и актуальнее данные, тем полезнее будет прогноз." },
    { title: "На какой срок можно посмотреть прогноз?", text: "В интерфейсе предусмотрены горизонты 30, 60 и 90 дней. Здесь на лендинге можно переключить период и изучить демонстрационный пример без регистрации." },
    { title: "Прогноз гарантирует, что денег хватит?", text: "Нет. Прогноз — оценка на основе исходных данных и допущений. Реальные поступления и расходы могут отличаться. Используйте его, чтобы заранее заметить возможный дефицит и проверить варианты действий." },
    { title: "Что сейчас работает в демонстрационном режиме?", text: "Графики и сценарии на этой странице используют синтетические данные и считаются локально. Опубликованная демонстрация приложения также работает с browser-local mock: без банковского подключения, реальной отправки писем и вызовов ML. Сквозная интеграция backend ещё не завершена." },
    { title: "Можно ли работать с командой и несколькими компаниями?", text: "В приложении есть экраны компаний, участников и ролей. Финансовые данные относятся к выбранной компании. Текущие MVP-контракты ориентированы на рубли и один подключённый счёт компании; мультивалютная консолидация не заявляется." },
];


const Landing = () => (
    <>
        <a className={styles.skipLink} href="#main">Перейти к содержимому</a>
        <Header />
        <main id="main">
            <section className={`${styles.container} ${styles.hero}`}>
                <div className={styles.heroCopy}>
                    <span className={styles.pill}><span />ФИНАНСОВЫЙ ПРОГНОЗ ДЛЯ БИЗНЕСА</span>
                    <h1>Знайте заранее,<br />когда бизнесу<br />нужны <span>деньги.</span></h1>
                    <p className={styles.heroDescription}>Сегодня — ясность.<br />Завтра — обдуманные решения.</p>
                    <p className={styles.heroText}>Поступления, платежи и прогноз остатка — в одной картине. Замечайте возможный кассовый разрыв до того, как он станет проблемой.</p>
                    <div className={styles.heroActions}><a href={appLinks.register} className={styles.primaryButton}>Начать работу<Icon name="arrow-right" /></a><a href="#demo" className={styles.textButton}><span className={styles.playIcon}>▶</span>Посмотреть пример</a></div>
                    <div className={styles.heroNote}><Icon name="check" size={15} />Для предпринимателей и финансовых команд</div>
                </div>
                <div className={styles.heroVisual}>
                    <div className={styles.heroOrbit} aria-hidden="true" />
                    <div className={styles.floatingLabel}><span className={styles.floatingIcon}><Icon name="chart-line" size={21} /></span><div><span>Видеть дальше</span><strong>Решать спокойнее</strong></div><span className={styles.sparkle}>✳</span></div>
                    <ForecastDemo />
                    <div className={styles.floatingBottom}><span><Icon name="calendar" size={18} /></span>Ваши планы уже на графике<Icon name="check" size={17} /></div>
                </div>
            </section>

            <div className={`${styles.container} ${styles.valueStrip}`}><span>МЕНЬШЕ НЕОПРЕДЕЛЁННОСТИ.<br /><strong>БОЛЬШЕ КОНТРОЛЯ.</strong></span><div><Icon name="calendar" />Платежи по плану</div><div><Icon name="chart-line" />Прогноз до 90 дней</div><div><Icon name="adjustments" />Проверка сценариев</div></div>

            <section id="features" className={`${styles.container} ${styles.section}`}>
                <div className={styles.sectionHeading} data-reveal><div><span className={styles.eyebrow}>ФИНАНСЫ БЕЗ СЛЕПЫХ ЗОН</span><h2>Весь денежный поток.<br /><span>Одна понятная картина.</span></h2></div><p>Не просто знать, сколько денег сейчас.<br />Понимать, что будет с ними дальше.</p></div>
                <div className={styles.bentoGrid}>
                    <article className={styles.featureForecast} data-reveal><span className={styles.cardNumber}>01 / ВИДЕТЬ НАПЕРЁД</span><h3>Кассовый разрыв<br />не должен быть сюрпризом.</h3><p>Увидьте дату и размер возможного дефицита. У вас будет время пересмотреть платежи и подготовиться.</p><div className={styles.miniForecast} aria-hidden="true"><div><span>ОЖИДАЕМЫЙ ОСТАТОК</span><strong>−42 000 ₽</strong></div><svg viewBox="0 0 500 130" fill="none"><path d="M0 25H500M0 80H500M0 125H500" stroke="currentColor" opacity=".12" /><path d="M0 25C30 25 40 8 70 20S110 65 145 55S185 35 215 60S245 80 275 75S305 128 335 118S380 80 405 87S460 48 500 43" stroke="currentColor" strokeWidth="3" /><circle cx="335" cy="118" r="6" fill="currentColor" /></svg><span className={styles.miniForecastLabel}>СЕГОДНЯ<span>ВОЗМОЖНЫЙ ДЕФИЦИТ</span></span></div><span className={styles.exampleLabel}>Иллюстрация на демонстрационных данных</span></article>
                    <article className={styles.featureCalendar} data-reveal><span className={styles.cardNumber}>02 / ПЛАНИРОВАТЬ ПЛАТЕЖИ</span><h3>Для каждого платежа —<br />своё время.</h3><p>Будущие поступления и обязательные расходы рядом. Планируйте с учётом реальных сроков.</p><div className={styles.calendarPreview}><div className={styles.calendarPreviewHead}><strong>Ноябрь</strong><span>Пример календаря<Icon name="calendar" size={16} /></span></div><div className={styles.calendarDays}>{ [ "ПН", "ВТ", "СР", "ЧТ", "ПТ", "СБ", "ВС" ].map(( day ) => <span key={day}>{ day }</span>) }{ Array.from({ length: 14 }, ( _, i ) => <span key={i} className={i === 4 ? styles.calendarSelected : i === 9 ? styles.calendarIncome : ""}>{ i + 2 }</span>) }</div><div className={styles.calendarPayment}><span className={styles.paymentDot} /><span>Аренда помещения</span><strong>−85 000 ₽</strong></div><div className={styles.calendarPayment}><span className={styles.incomeDot} /><span>Оплата от клиента</span><strong>+180 000 ₽</strong></div></div></article>
                    <article className={styles.featureSmall} data-reveal><span className={styles.smallIcon}><Icon name="file-upload" size={27} /></span><div><h3>От операций — к порядку</h3><p>История, категории и импорт с проверкой перед применением.</p></div><Icon name="arrow-right" /></article>
                    <article className={`${styles.featureSmall} ${styles.featureTeam}`} data-reveal><span className={styles.smallIcon}><Icon name="users" size={27} /></span><div><h3>Одна команда. Общая картина.</h3><p>Компании, участники и роли для совместной работы с финансами.</p></div><Icon name="arrow-right" /></article>
                </div>
                <p className={styles.sectionFootnote}><Icon name="info-circle" size={16} />Возможности представлены в интерфейсе приложения. Текущая публичная версия работает в демонстрационном режиме.</p>
            </section>

            <section id="how-it-works" className={styles.howSection}><div className={styles.container}><div className={styles.sectionHeading} data-reveal><div><span className={styles.eyebrow}>ОТ ДАННЫХ К РЕШЕНИЮ</span><h2>Первый взгляд вперёд —<br /><span>за четыре шага.</span></h2></div><a className={styles.textButton} href={appLinks.register}>Перейти к регистрации<Icon name="arrow-right" /></a></div><div className={styles.steps}>{ steps.map(( step ) => <article key={step.number} data-reveal><div className={styles.stepTop}><span>{ step.number }</span><Icon name={step.icon} size={24} /></div><h3>{ step.title }</h3><p>{ step.text }</p></article>) }</div></div></section>

            <section id="scenarios" className={`${styles.container} ${styles.section}`}><div className={styles.sectionHeading} data-reveal><div><span className={styles.eyebrow}>СНАЧАЛА ПРОВЕРЬТЕ. ПОТОМ РЕШАЙТЕ.</span><h2>А что, если<br /><span>всё пойдёт иначе?</span></h2></div><p>Клиент задержал оплату. Выручка снизилась.<br />Посмотрите, как это меняет картину.</p></div><div data-reveal><ScenarioDemo /></div></section>

            <section className={`${styles.container} ${styles.statement}`} data-reveal><Image src="/icons/saldo/saldo-symbol-green.svg" alt="" width={92} height={92} unoptimized /><p>Вы управляете бизнесом.<br /><span>Saldo помогает видеть его финансовое завтра.</span></p><span className={styles.statementMark} aria-hidden="true">↗</span></section>

            <section id="faq" className={`${styles.container} ${styles.faqSection}`}><div data-reveal><span className={styles.eyebrow}>ЕСТЬ ВОПРОСЫ?</span><h2>Давайте<br /><span>разберёмся.</span></h2><p>Самое важное перед первым прогнозом.</p></div><div className={styles.questions}>{ questions.map(( item, index ) => <details key={item.title} className={styles.question} data-reveal><summary><span className={styles.questionIndex}>0{ index + 1 }</span><span>{ item.title }</span><span className={styles.questionPlus} aria-hidden="true">+</span></summary><p>{ item.text }</p></details>) }</div></section>

            <section className={`${styles.container} ${styles.finalCta}`} data-reveal><div className={styles.ctaDecor} aria-hidden="true">↗</div><span className={styles.pill}><span />СМОТРИТЕ НА ФИНАНСЫ ВПЕРЁД</span><h2>Завтра начинается<br /><span>с ясности сегодня.</span></h2><p>Соберите денежные потоки в одну картину<br />и принимайте решения с пониманием будущего остатка.</p><a href={appLinks.register} className={styles.primaryButton}>Начать работу с Saldo<Icon name="arrow-right" /></a><span className={styles.ctaLogin}>Уже знакомы с Saldo? <a href={appLinks.login}>Войти в аккаунт ↗</a></span></section>
            <div className={`${styles.container} ${styles.footerBridge}`}><a href="#" aria-label="Saldo Analytics — наверх"><Image src="/icons/saldo/saldo-analytics-black.svg" alt="Saldo Analytics" width={140} height={52} unoptimized /></a><p>Финансовая ясность для вашего бизнеса.</p><a href="#main">Наверх ↑</a></div>
        </main>
        <Footer />
        <Reveal />
    </>
);


export default Landing;
