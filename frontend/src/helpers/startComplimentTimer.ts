const compliments = [
    "Отлично выглядите!",
    "У вас всё получится!",
    "Ваши идеи заслуживают воплощения!",
    "Вы делаете этот день лучше!",
    "У вас отличный вкус!",
    "Не забывайте: вы молодец!",
    "Ваше внимание к деталям впечатляет!",
    "Вы умеете находить хорошие решения!",
    "С вами даже цифры становятся интереснее!",
    "Ваше упорство заслуживает уважения!",
    "Вы движетесь в правильном направлении!",
    "У вас прекрасное чувство стиля!",
    "Ваши старания уже имеют значение!",
    "Вы умеете смотреть на шаг вперёд!",
    "У вас замечательные идеи!",
    "Вы заслужили минутку для себя!",
    "Ваша любознательность вдохновляет!",
    "Вы отлично справляетесь!",
    "Сегодня хороший день для ваших планов!",
    "Вы добавляете этому дню уверенности!",
];


const getCompliment = () => {
    const value = Math.random();

    if (value < 0.4) {
        return compliments[0];
    }

    const count = compliments.length - 1;
    const index = Math.min(count - 1, Math.floor((value - 0.4) / 0.6 * count));

    return compliments[index + 1];
};


const startComplimentTimer = ( onCompliment: ( message: string ) => void ) => {
    let timer: ReturnType<typeof setTimeout>;


    const showCompliment = () => {
        onCompliment(getCompliment());

        const delay = (5 + Math.random() * 115) * 60_000;

        timer = setTimeout(showCompliment, delay);
    };


    timer = setTimeout(showCompliment, 3 * 60_000);

    return () => clearTimeout(timer);
};


export { getCompliment };
export default startComplimentTimer;
