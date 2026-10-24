interface IFooterLink {
    title: string;
    href: string;
}


interface IFooterGroup extends IFooterLink {
    links: IFooterLink[];
}


const bankUrl = ( path: string ) => path.startsWith("/") ? `https://www.centrinvest.ru${path}` : path;


const groups: IFooterGroup[] = [
    {
        title: "Частным лицам", href: "/",
        links: [
            { title: "Кредитование", href: "/for-individuals/crediting/catalog" },
            { title: "Ипотека", href: "/for-individuals/crediting/catalog?filter=mortgage" },
            { title: "Вклады", href: "/for-individuals/deposits" },
            { title: "Карты", href: "/for-individuals/cards/debet-cards" },
            { title: "VIP-обслуживание", href: "/for-individuals/vip-service" },
            { title: "Центр-инвест Онлайн", href: "/for-individuals/centr-invest-online" },
        ],
    },
    {
        title: "Бизнесу", href: "/business",
        links: [
            { title: "Открытие и ведение счета", href: "/business/opening-and-maintaining-account" },
            { title: "Кредитование", href: "/business/business-crediting" },
            { title: "Эквайринг", href: "/business/acquiring" },
            { title: "Зарплатные проекты", href: "/business/salary-projects" },
            { title: "Депозиты", href: "/business/placement-of-funds-and-securities" },
            { title: "Самозанятым", href: "/business/self-employed" },
            { title: "Банковские гарантии", href: "/business/business-crediting/bankovskie-garantii" },
        ],
    },
    {
        title: "О банке", href: "/about",
        links: [
            { title: "Контакты", href: "/about/contacts" },
            { title: "Акционерам и инвесторам", href: "/about/investors" },
            { title: "Офисы и банкоматы", href: "/about/offices-and-atms" },
            { title: "Обращение в банк", href: "/cabinet/internet-reception/info" },
            { title: "Блог", href: "/about/blog" },
        ],
    },
    {
        title: "Социальные проекты", href: "/social-projects",
        links: [
            { title: "Медицинским работникам", href: "/social-projects/medical-professionals" },
            { title: "Студентам", href: "/for-individuals/cards/student-cards" },
            { title: "Пенсионерам", href: "/for-individuals/cards/debet-cards/pens" },
            { title: "Финансовая грамотность", href: "https://xn----7sbbzgbvlb6b9j.xn--p1ai/" },
            { title: "Карьера", href: "/social-projects/career" },
        ],
    },
    {
        title: "ESG", href: "/esg",
        links: [
            { title: "Зелёный вклад", href: "/for-individuals/deposits/green" },
            { title: "Зелёная ипотека", href: "/for-individuals/mortgage/green-mortgage" },
            { title: "Зелёный вектор", href: "https://greenvector.media" },
            { title: "Социальный вклад банка", href: "/about#about-bank-social" },
            { title: "ESG-библиотека", href: "/esg/esg-library" },
        ],
    },
];


const legalLinks: IFooterLink[] = [
    { title: "Информация о максимальных процентных ставках по вкладам физических лиц", href: "/about/investors/regulatory-purp" },
    { title: "Реквизиты", href: "/files/about/requisites.pdf" },
    { title: "Надзорный орган", href: "/nadzorniy_organ" },
    { title: "Раскрытие информации на сайте ИА Интерфакс", href: "https://e-disclosure.ru/portal/company.aspx?id=3018&attempt=3" },
    { title: "Реализация имущества", href: "https://zalog-market.ru/" },
    { title: "Уведомление физических лиц о необходимости представления сведений (документов) для целей исполнения законодательства о ПОД/ФТ", href: "/presentation-information" },
    { title: "Страхование вкладов", href: "/deposit-insurance" },
    { title: "Оценка условий труда", href: "/files/about/statement-working-conditions.pdf" },
    { title: "Генеральная лицензия Банка России № 2225 от 26 августа 2016 г.", href: "/files/about/license.pdf" },
    { title: "Соглашение об использовании информации на сайте", href: "/agreement" },
    { title: "Раскрытие информации", href: "/about/investors/pub-info" },
    { title: "Раскрытие информации профессионального участника рынка ценных бумаг", href: "/about/investors/pub-info" },
    { title: "Финансовый уполномоченный", href: "/financial-commissioner" },
];


const socialLinks = [
    { title: "ВКонтакте", href: "https://vk.ru/centrinvest", icon: "vk" },
    { title: "Одноклассники", href: "https://ok.ru/centrinvest.bank", icon: "classmates" },
    { title: "YouTube", href: "https://www.youtube.com/user/centrinvest", icon: "youtube" },
    { title: "MAX", href: "https://max.ru/centrinvest", icon: "max" },
    { title: "Для частных лиц", href: "https://t.me/centrinvest_bank", icon: "telegram" },
    { title: "Центр-инвест. Бизнес", href: "https://t.me/centrinvest_business", icon: "telegram" },
];


const contactLinks = [
    { title: "Контакты банка", href: "/about/contacts", icon: "feedback" },
    { title: "Написать ВКонтакте", href: "https://vk.ru/im?sel=-29049863", icon: "vk" },
    { title: "Написать в Одноклассниках", href: "https://ok.ru/group/52970451894358/messages", icon: "classmates" },
    { title: "Интернет-приёмная", href: "/cabinet/internet-reception/info", icon: "internet-reception" },
];


const apps = [
    { title: "App Store", href: "/for-individuals/centr-invest-online/instruction/install-ios", icon: "app-store-ios" },
    { title: "Google Play", href: "https://play.google.com/store/apps/details?id=ru.centrinvest.mobilebanking2018&hl=ru", icon: "google" },
    { title: "NashStore", href: "https://store.nashstore.ru/store/62cc37a00a39b24f4f8185b1", icon: "nashstore" },
    { title: "RuStore", href: "https://apps.rustore.ru/app/ru.centrinvest.mobilebanking2018", icon: "rustore" },
    { title: "AppGallery", href: "https://appgallery.huawei.com/app/C107165091", icon: "huawei" },
];


export { bankUrl, groups, legalLinks, socialLinks, contactLinks, apps };
