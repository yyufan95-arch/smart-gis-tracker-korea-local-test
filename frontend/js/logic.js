let steps = ['A', 'B', 'C', 'D'];
let currentIndex = 0;

function processCert() {
    if (currentIndex >= steps.length) {
        alert("모든 미션이 완료되었습니다!");
        return;
    }

    let currentStep = steps[currentIndex];
    const time = new Date().toLocaleTimeString();

    // --- 데이터 암호화 로직 ---
    // 단계명과 시간을 합쳐 Base64 인코딩 후 문자열을 뒤집는 방식으로 암호화
    let rawData = currentStep + "_" + time;
    let encryptedToken = btoa(rawData).split("").reverse().join("");

    // --- 로컬 스토리지를 이용한 가상 데이터베이스 연동 ---
    let db = JSON.parse(localStorage.getItem('smart_gis_db') || "[]");
    db.push({
        step: currentStep,
        time: time,
        token: encryptedToken,
        status: "Certified"
    });
    localStorage.setItem('smart_gis_db', JSON.stringify(db));

    // --- 화면 UI 갱신 처리 ---
    // 1. 완료된 단계 스타일 변경 및 체크 표시
    let li = document.getElementById('li-' + currentStep);
    li.classList.add('completed');
    li.innerHTML = currentStep + ". " + getStepName(currentStep) + " ✓";

    // 2. 로그 영역에 데이터 출력
    document.getElementById('log').innerText = JSON.stringify(db, null, 2);

    // 3. 다음 단계 안내 메시지 갱신
    currentIndex++;
    if (currentIndex < steps.length) {
        document.getElementById('current-task').innerText = "当前状态：请前往 " + steps[currentIndex] + " 点认证";
    } else {
        document.getElementById('current-task').innerText = "状态：우석대 캠퍼스 전체 미션 완료!";
        document.getElementById('certifyBtn').disabled = true;
        document.getElementById('certifyBtn').style.background = "#ccc";
    }

    alert("인증 성공! 위치 데이터가 가상 DB에 암호화되어 동기화되었습니다.");
}

// 단계별 명칭 매칭 
function getStepName(id) {
    const names = {
        'A': '문화관 인증',
        'B': '정궁관 인증',
        'C': '체육관 인증',
        'D': '학생관 인증 완료'
    };
    return names[id];
}

// 페이지 진입 시 기존 데이터 초기화 (시연용)
window.onload = () => {
    localStorage.removeItem('smart_gis_db');
    console.log("시연 시작: 가상 데이터베이스를 초기화했습니다.");
};
