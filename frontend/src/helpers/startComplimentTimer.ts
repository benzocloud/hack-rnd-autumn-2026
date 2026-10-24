const compliments = [
    "Отлично выглядите!",
    "У вас всё получится!",
    "Ваши идеи заслуживают воплощения!",
    "Вы делаете этот день лучше!",
    "У вас отличный вкус!",
    "Не забывайте: вы молодец!",
];


const startComplimentTimer = ( onCompliment: ( message: string ) => void ) => {
    let timer: ReturnType<typeof setTimeout>;


    const showCompliment = () => {
        onCompliment(compliments[Math.floor(Math.random() * compliments.length)]);

        const delay = (5 + Math.random() * 115) * 60_000;

        timer = setTimeout(showCompliment, delay);
    };


    timer = setTimeout(showCompliment, 3 * 60_000);

    return () => clearTimeout(timer);
};


export default startComplimentTimer;
